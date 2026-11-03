package inspect_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// projectionServer serves the inspector over a freshly seeded ledger in the test's own
// temp directory.
func projectionServer(t *testing.T) *httptest.Server {
	t.Helper()
	l, err := seedProjectionLedger(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("seed projection fixture: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Catalog: catalogtest.Engine.Catalog(t), Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(srv.Close)
	return srv
}

// seedProjectionLedger builds the fixture: a facet the measures template serves, a live one
// it excludes by type, one that has been corrected (so a version is displaced), and one
// that has been tombstoned. The real ledger has no corrections in it, so this is the
// only place the superseded arm is exercised.
//
// Anam-times are spread across two days so the as-of control has a boundary to land
// between.
func seedProjectionLedger(path string) (*ledger.Ledger, error) {
	ctx := context.Background()

	l, err := ledger.Create(ctx, path, person)
	if err != nil {
		return nil, fmt.Errorf("ledger.Create: %w", err)
	}

	at := func(seq int64, day int) facet.AnamEvent {
		return facet.AnamEvent{AnamTime: time.Date(2026, 7, day, 9, 0, 0, 0, time.UTC), Seq: seq}
	}

	// seq 1 — a measure, admitted on the 10th.
	measure := seedFacetNoT("meter:2026-01-24T06:30:00", "test.measure", `{"count": 118}`)
	ev := at(1, 10)
	ev.Kind, ev.FacetID, ev.ArtifactVersion = facet.EventAdmitted, measure.ID, 1
	if err := l.Admit(ctx, 0, measure, ev); err != nil {
		return nil, fmt.Errorf("admit measure: %w", err)
	}

	// seq 2 — a reading, which measures excludes by type.
	reading := seedFacetNoT("logger:2026-07-05", "test.reading", `{"value": 54}`)
	ev = at(2, 10)
	ev.Kind, ev.FacetID, ev.ArtifactVersion = facet.EventAdmitted, reading.ID, 1
	if err := l.Admit(ctx, 1, reading, ev); err != nil {
		return nil, fmt.Errorf("admit reading: %w", err)
	}

	// seq 3 — the measure is corrected on the 12th: v1 is displaced by v2.
	corrected := seedFacetNoT("meter:2026-01-24T06:30:00", "test.measure", `{"count": 117}`)
	corrected.ArtifactVersion = 2
	ev = at(3, 12)
	ev.Kind, ev.FacetID, ev.ArtifactVersion, ev.PriorArtifactVersion = facet.EventSuperseded, corrected.ID, 2, 1
	if err := l.Admit(ctx, 2, corrected, ev); err != nil {
		return nil, fmt.Errorf("admit correction: %w", err)
	}

	// seq 4 — the reading is retracted on the 12th.
	ev = at(4, 12)
	ev.Kind, ev.FacetID, ev.ArtifactVersion = facet.EventTombstoned, reading.ID, 1
	if err := l.Mark(ctx, 3, ev); err != nil {
		return nil, fmt.Errorf("mark tombstone: %w", err)
	}

	// seq 5 — a course start, admitted on the 12th and left live. A *live* facet of
	// another type is what exercises the template's type exclusion: the tombstoned
	// reading above never reaches a template at all, because the canonical is
	// live-only by construction.
	course := seedFacetNoT("records:course:2026-07-05", "test.course.started", `{"id": "c1", "date": "2026-07-05", "name": "a", "course": 1, "dose": "1"}`)
	ev = at(5, 12)
	ev.Kind, ev.FacetID, ev.ArtifactVersion = facet.EventAdmitted, course.ID, 1
	if err := l.Admit(ctx, 4, course, ev); err != nil {
		return nil, fmt.Errorf("admit course: %w", err)
	}

	return l, nil
}

// The canonical view is the un-narrowed read: every live facet stands served, and the
// versions no longer in force are still accounted for rather than quietly dropped.
func TestProjectionPage_Canonical(t *testing.T) {
	t.Parallel()
	srv := projectionServer(t)

	code, body := get(t, srv.URL+"/projection")
	if code != 200 {
		t.Fatalf("GET /projection = %d\n%s", code, body)
	}
	for _, want := range []string{"anam projections", "canonical", "measures", "course", "as-of", "superseded"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not mention %q", want)
		}
	}
	// The summary counts the versions no longer in force; it does not list them.
	if strings.Contains(body, "displaced by v2") {
		t.Error("the summary drew a superseded row")
	}

	// One live measure (at v2), one tombstoned reading: the displaced v1 is the measure's
	// superseded row, and the dead reading is the (not live) line's.
	_, rows := get(t, srv.URL+"/projection?type=test.measure&show=superseded")
	if !strings.Contains(rows, "displaced by v2") {
		t.Errorf("the displaced version is not explained:\n%s", rows)
	}
	_, dead := get(t, srv.URL+"/projection?type="+url.QueryEscape("(not live)")+"&show=superseded")
	if !strings.Contains(dead, "not live: tombstoned") {
		t.Errorf("the tombstoned facet is not explained:\n%s", dead)
	}
}

// Switching the template must change the verdict on every facet, and say why.
func TestProjectionPage_SwitchingTemplateChangesTheVerdicts(t *testing.T) {
	t.Parallel()
	srv := projectionServer(t)

	code, measures := get(t, srv.URL+"/projection?p=measures")
	if code != 200 {
		t.Fatalf("GET /projection?p=measures = %d", code)
	}
	_, measuresRows := get(t, srv.URL+"/projection?p=measures&type=test.course.started&show=excluded")
	// The live course facet is what the type knob excludes here. The tombstoned
	// reading never reaches the template at all — the canonical is live-only by
	// construction, so a dead facet is a superseded row, not an exclusion.
	if !strings.Contains(measuresRows, "type not in the allow-list") {
		t.Errorf("measures does not explain why the live course facet is out:\n%s", measuresRows)
	}
	if !strings.Contains(measures, "test.measure") {
		t.Error("measures does not show the measure it serves")
	}

	// The same ledger through the other template: the verdicts swap over. That swap
	// IS the demonstration of governance — one read, two governed variants, and the
	// difference is visible without re-running anything.
	code, courseBody := get(t, srv.URL+"/projection?p=course")
	if code != 200 {
		t.Fatalf("GET /projection?p=course = %d", code)
	}
	_, courseRows := get(t, srv.URL+"/projection?p=course&type=test.measure&show=excluded")
	if !strings.Contains(courseRows, "type not in the allow-list") {
		t.Error("course does not explain its exclusions")
	}
	if measures == courseBody {
		t.Error("switching the template changed nothing on the page")
	}

	// The served hashes must differ: they are different values, so they are
	// different content addresses.
	if servedHash(t, measures) == servedHash(t, courseBody) {
		t.Error("measures and course served the same hash over a ledger holding both types")
	}

	// And the hash is identified as the receipt's — that is what makes the viewer
	// checkable against the publisher rather than merely plausible.
	if !strings.Contains(measures, "a receipt would carry") {
		t.Error("the served hash is not identified as the receipt's")
	}
	if !strings.Contains(measures, "measures v1") {
		t.Error("the template version is not shown")
	}
}

// servedHash pulls the served content address out of the rendered page.
func servedHash(t *testing.T, body string) string {
	t.Helper()
	const marker = "served <code>"
	i := strings.Index(body, marker)
	if i < 0 {
		t.Fatalf("no served hash on the page")
	}
	rest := body[i+len(marker):]
	j := strings.Index(rest, "</code>")
	if j < 0 {
		t.Fatalf("malformed served hash")
	}
	return rest[:j]
}

// The as-of control is the whole point of "select a time point": before the correction
// landed, the projection stood on v1, and the page must say v1 rather than today's v2.
func TestProjectionPage_AsOfShowsWhatItStoodOnThen(t *testing.T) {
	t.Parallel()
	srv := projectionServer(t)

	// The 11th: after the first two admissions, before the correction and tombstone.
	code, before := get(t, srv.URL+"/projection?p=measures&asof=2026-07-11T00:00:00Z")
	if code != 200 {
		t.Fatalf("as-of GET = %d\n%s", code, before)
	}
	if !strings.Contains(before, ">2</strong> watermark") && !strings.Contains(before, "<strong>2</strong>") {
		t.Errorf("as-of the 11th should read W=2:\n%s", before)
	}
	// Nothing had been displaced or tombstoned yet, so no type has a superseded count
	// to open; now both have happened.
	if got := summaryCounts(t, before); got["test.measure"].superseded != 0 || got[notLiveType].link != "" {
		t.Errorf("as-of the 11th the summary counts superseded versions: %+v", got)
	}
	_, now := get(t, srv.URL+"/projection?p=measures")
	if got := summaryCounts(t, now); got["test.measure"].superseded != 1 || got[notLiveType].superseded != 1 {
		t.Errorf("the head summary lost the supersession or the tombstone: %+v", got)
	}
	// The as-of survives into the rows view, which then reads the same past.
	_, thenRows := get(t, srv.URL+"/projection?p=measures&type=test.measure&show=served&asof=2026-07-11T00:00:00Z")
	if !strings.Contains(thenRows, ">v1<") || strings.Contains(thenRows, ">v2<") {
		t.Errorf("as-of the 11th the rows view should show the reading at v1:\n%s", thenRows)
	}

	// And before anything was admitted, the honest answer is an empty projection
	// rather than an error — a receipt could legitimately name that watermark.
	code, genesis := get(t, srv.URL+"/projection?p=measures&asof=2026-01-01T00:00:00Z")
	if code != 200 {
		t.Fatalf("as-of before genesis = %d", code)
	}
	if !strings.Contains(genesis, "Anam knew nothing yet") {
		t.Errorf("an empty as-of read should say so:\n%s", genesis)
	}
}

func TestProjectionPage_RejectsBadInput(t *testing.T) {
	t.Parallel()
	srv := projectionServer(t)

	if code, _ := get(t, srv.URL+"/projection?p=measure"); code != 404 {
		t.Errorf("unknown projection = %d, want 404", code)
	}
	// The form's datetime-local value carries seconds and no zone; each spelling
	// reads the same instant as the RFC 3339 one, so the page is the same apart
	// from the value echoed back into the input.
	// The summary's links carry the as-of as typed, so they differ by spelling too.
	asofParam := regexp.MustCompile(`asof=[^&"]*`)
	same := func(body, asof string) string {
		return asofParam.ReplaceAllString(strings.ReplaceAll(body, `value="`+asof+`"`, ""), "asof=")
	}
	_, ref := get(t, srv.URL+"/projection?p=measures&asof=2026-07-11T00:00:00Z")
	for _, local := range []string{"2026-07-11T00:00:00", "2026-07-11T00:00"} {
		code, body := get(t, srv.URL+"/projection?p=measures&asof="+local)
		if code != 200 || same(body, local) != same(ref, "2026-07-11T00:00:00Z") {
			t.Errorf("asof=%s: %d, and differs from the RFC 3339 spelling", local, code)
		}
	}
	if code, _ := get(t, srv.URL+"/projection?asof=last-tuesday"); code != 400 {
		t.Errorf("unparseable as-of = %d, want 400", code)
	}
	for _, bad := range []string{"type=test.course.started&show=bogus", "type=test.course.started&from=yesterday", "type=test.course.started&to=2026-13-40"} {
		if code, _ := get(t, srv.URL+"/projection?"+bad); code != 400 {
			t.Errorf("%s = %d, want 400", bad, code)
		}
	}
}
