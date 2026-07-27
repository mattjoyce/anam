package fold_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/fold"
)

// WatermarkAt is the bridge between the two clocks: a person asks "what did this show
// me on Tuesday?" in anam-time, and every read below that line is in watermarks.
//
// The ingests are separated in wall-clock time here so the boundaries are real rather
// than an artefact of everything landing in the same instant.
func TestWatermarkAt_TranslatesAnamTimeToWatermark(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	before := time.Now().UTC()

	source{"2026-01-24T06:30:00Z": `"systolic": 118, "diastolic": 76`}.ingest(t, g)
	firstW, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	between := time.Now().UTC()
	time.Sleep(20 * time.Millisecond)

	source{
		"2026-01-25T06:30:00Z": `"systolic": 121, "diastolic": 79`,
		"2026-01-26T06:30:00Z": `"systolic": 117, "diastolic": 75`,
	}.ingest(t, g)
	head, err := l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	cases := []struct {
		name string
		at   time.Time
		want int64
	}{
		{"before anything was admitted", before.Add(-time.Hour), 0},
		{"after the first ingest", between, int64(firstW)},
		{"after the second", after, int64(head)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := fold.WatermarkAt(ctx, l, c.at)
			if err != nil {
				t.Fatalf("WatermarkAt: %v", err)
			}
			if int64(got) != c.want {
				t.Errorf("WatermarkAt(%s) = %d, want %d", c.at, got, c.want)
			}
		})
	}
}

// The point of the translation: reading at a past anam-time shows what Anam knew
// then, not what it knows now.
func TestWatermarkAt_ComposesIntoAnAsOfRead(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	src := source{"2026-01-24T06:30:00Z": `"systolic": 118, "diastolic": 76`}
	src.ingest(t, g)
	time.Sleep(20 * time.Millisecond)
	cut := time.Now().UTC()
	time.Sleep(20 * time.Millisecond)

	// The world moves on: one correction, one new reading.
	src["2026-01-24T06:30:00Z"] = `"systolic": 116, "diastolic": 74`
	src["2026-01-25T06:30:00Z"] = `"systolic": 121, "diastolic": 79`
	src.ingest(t, g)

	w, err := fold.WatermarkAt(ctx, l, cut)
	if err != nil {
		t.Fatalf("WatermarkAt: %v", err)
	}
	asOf, err := fold.ReadAt(ctx, l, w)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if asOf.Value.Len() != 1 {
		t.Fatalf("as-of held %d facets, want the 1 Anam knew then", asOf.Value.Len())
	}
	for _, e := range asOf.Value.Entries() {
		if got := string(e.Payload); !strings.Contains(got, `"systolic":118`) {
			t.Errorf("as-of returned the corrected reading, not the one in force: %s", got)
		}
	}

	now, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if now.Value.Len() != 2 {
		t.Errorf("at head: %d facets, want 2", now.Value.Len())
	}
}

// Displaced is the other half of "which events contribute to this projection": the
// versions that no longer do. It is shared by the CLI's --explain and the inspector's
// projection page, so both give the same answer by construction rather than by two
// authors agreeing.
func TestDisplaced_ListsEveryVersionNoLongerInForce(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	// A reading corrected twice, plus one never corrected.
	src := source{
		"2026-01-24T06:30:00Z": `"systolic": 118, "diastolic": 76`,
		"2026-01-25T06:30:00Z": `"systolic": 121, "diastolic": 79`,
	}
	src.ingest(t, g)
	src["2026-01-24T06:30:00Z"] = `"systolic": 117, "diastolic": 75`
	src.ingest(t, g)
	src["2026-01-24T06:30:00Z"] = `"systolic": 116, "diastolic": 74`
	src.ingest(t, g)

	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if res.Value.Len() != 2 {
		t.Fatalf("live = %d, want 2", res.Value.Len())
	}

	got, err := fold.Displaced(ctx, l, res)
	if err != nil {
		t.Fatalf("Displaced: %v", err)
	}
	// The corrected facet reached v3, so v1 and v2 are displaced. The other is at v1,
	// so nothing of it is.
	if len(got) != 2 {
		t.Fatalf("Displaced returned %d, want 2 (v1 and v2 of the corrected reading)", len(got))
	}
	for i, want := range []struct {
		version int
		reason  string
	}{{1, "displaced by v2"}, {2, "displaced by v3"}} {
		if got[i].Version != want.version || got[i].Reason != want.reason {
			t.Errorf("Displaced[%d] = v%d %q, want v%d %q",
				i, got[i].Version, got[i].Reason, want.version, want.reason)
		}
	}

	// The displaced artifacts hold the *old* content — that is the whole point of
	// being able to show them rather than count them.
	if got := string(got[0].Facet.Payload.Bytes()); !strings.Contains(got, `"systolic":118`) {
		t.Errorf("v1 payload = %s, want the original reading", got)
	}
	if got := string(got[1].Facet.Payload.Bytes()); !strings.Contains(got, `"systolic":117`) {
		t.Errorf("v2 payload = %s, want the first correction", got)
	}

	// A person with no corrections pays nothing here.
	clean, cleanL := newHarness(t)
	source{"2026-02-01T06:30:00Z": `"systolic": 120, "diastolic": 78`}.ingest(t, clean)
	cleanRes, err := fold.Read(ctx, cleanL)
	if err != nil {
		t.Fatalf("Read (clean): %v", err)
	}
	cleanDisplaced, err := fold.Displaced(ctx, cleanL, cleanRes)
	if err != nil {
		t.Fatalf("Displaced (clean): %v", err)
	}
	if len(cleanDisplaced) != 0 {
		t.Errorf("an uncorrected ledger reported %d displaced versions", len(cleanDisplaced))
	}
}

// A retracted facet's current version is not in force either, and saying so is what
// makes "what did this projection stand on" answerable — pretending retracted facts
// were never there would be a different, false answer.
func TestDisplaced_IncludesTheDeadCurrentVersion(t *testing.T) {
	t.Parallel()
	g, l := newHarness(t)
	ctx := context.Background()

	results := source{"2026-01-24T06:30:00Z": `"systolic": 118, "diastolic": 76`}.ingest(t, g)
	var id string
	for _, r := range results {
		id = r.FacetID
	}
	if _, _, err := g.Retract(ctx, id); err != nil {
		t.Fatalf("Retract: %v", err)
	}

	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if res.Value.Len() != 0 {
		t.Fatalf("a retracted facet is still live: %d", res.Value.Len())
	}

	got, err := fold.Displaced(ctx, l, res)
	if err != nil {
		t.Fatalf("Displaced: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Displaced returned %d, want the 1 retracted version", len(got))
	}
	if got[0].Reason != "not live: tombstoned" {
		t.Errorf("reason = %q, want %q", got[0].Reason, "not live: tombstoned")
	}
	if got[0].Version != 1 {
		t.Errorf("version = %d, want 1", got[0].Version)
	}
}
