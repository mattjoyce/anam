package inspect_test

import (
	"context"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// The strip under the band compares each bucket's value at W with the head the log
// holds (card 21): a real ledger, a real operational store and a real mint, so the
// three states come from the same hash comparison the mint makes.

func seedAt(sourceRef, ftype, payload string, at time.Time) facet.Facet {
	f := seedFacetNoT(sourceRef, ftype, payload)
	f.FactTime = at
	return f
}

type checkpointFixture struct {
	l    *ledger.Ledger
	deps mint.Deps
	srv  *httptest.Server
	seq  int64
}

func (f *checkpointFixture) admit(t *testing.T, fc facet.Facet) {
	t.Helper()
	f.seq++
	if err := f.l.Admit(context.Background(), ledger.Watermark(f.seq-1), fc, event(facet.EventAdmitted, fc, f.seq)); err != nil {
		t.Fatalf("Admit %s: %v", fc.Provenance.SourceRef, err)
	}
}

func (f *checkpointFixture) mint(t *testing.T) {
	t.Helper()
	if _, err := mint.Run(context.Background(), f.deps, false); err != nil {
		t.Fatalf("mint.Run: %v", err)
	}
}

func newCheckpointFixture(t *testing.T) *checkpointFixture {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	db, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), defaultBuckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := artifact.New(t.TempDir())
	if err != nil {
		t.Fatalf("artifact.New: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Checkpoints: db}))
	t.Cleanup(srv.Close)
	return &checkpointFixture{
		l: l, srv: srv,
		deps: mint.Deps{
			Ledger: l, Index: db, Artifacts: store, Buckets: defaultBuckets,
			Clock: clock.Fixed{Instant: time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)},
		},
	}
}

func TestTimeline_BucketStripFollowsTheMint(t *testing.T) {
	t.Parallel()
	f := newCheckpointFixture(t)
	july := time.Date(2026, 7, 5, 6, 30, 0, 0, time.UTC)
	august := time.Date(2026, 8, 5, 6, 30, 0, 0, time.UTC)
	f.admit(t, seedAt("bp-csv:2026-07-05", "health.blood-pressure", `{"systolic": 118, "diastolic": 76}`, july))
	f.admit(t, seedAt("bp-csv:2026-08-05", "health.blood-pressure", `{"systolic": 121, "diastolic": 79}`, august))
	f.mint(t)

	code, body := get(t, f.srv.URL+"/?view=whole")
	if code != 200 {
		t.Fatalf("GET /: %d\n%s", code, body)
	}
	for _, want := range []string{"2 buckets, 2 sealed at W = 2", "last mint run at W = 2",
		`class="bucket sealed"`, "2026-07 · sealed · v1 covers W=2 · 1 live facet", "2026-08 · sealed · v1"} {
		if !strings.Contains(body, want) {
			t.Errorf("after a mint the page lacks %q", want)
		}
	}
	summary := excerpt(body, "cp-summary")
	if strings.Contains(summary, "behind") || strings.Contains(summary, "never minted") || strings.Contains(summary, "unsealed") ||
		strings.Contains(body, `class="bucket behind"`) || strings.Contains(body, `class="bucket never"`) {
		t.Errorf("a freshly minted record shows a bucket not sealed:\n%s", summary)
	}

	// A new reading in a minted month leaves its bucket behind; one in a month the
	// mint never saw is never minted. Both are correct floors, neither is wrong.
	f.admit(t, seedAt("bp-csv:2026-07-06", "health.blood-pressure", `{"systolic": 119, "diastolic": 77}`, july.Add(24*time.Hour)))
	f.admit(t, seedAt("bp-csv:2026-09-05", "health.blood-pressure", `{"systolic": 117, "diastolic": 75}`, time.Date(2026, 9, 5, 6, 30, 0, 0, time.UTC)))

	code, body = get(t, f.srv.URL+"/?view=whole")
	if code != 200 {
		t.Fatalf("GET /: %d\n%s", code, body)
	}
	for _, want := range []string{"3 buckets, 1 sealed, ", "1 behind", "1 never minted", "at W = 4", "last mint run at W = 2",
		`class="bucket behind"`, `class="bucket never"`,
		"2026-07 · behind: the value at W=4 differs from v1 (covers W=2) · 2 live facets",
		"2026-09 · never minted · 1 live facet", `href="/?from=2026-09-01T00%3A00%3A00Z`} {
		if !strings.Contains(body, want) {
			t.Errorf("after new readings the page lacks %q\n%s", want, excerpt(body, "cp-summary"))
		}
	}

	// The next mint seals everything again.
	f.mint(t)
	_, body = get(t, f.srv.URL+"/?view=whole")
	if !strings.Contains(body, "3 buckets, 3 sealed at W = 4") || !strings.Contains(body, "last mint run at W = 4") {
		t.Errorf("after the second mint:\n%s", excerpt(body, "cp-summary"))
	}
}

func TestTimeline_NoLogNoStrip(t *testing.T) {
	t.Parallel()
	srv, _, _ := seededServer(t)
	_, body := get(t, srv.URL+"/?view=whole")
	if strings.Contains(body, `class="buckets"`) || strings.Contains(body, "cp-summary") {
		t.Error("an inspector given no checkpoint log drew the strip")
	}
}

func TestTimeline_NeverMintedLedger(t *testing.T) {
	t.Parallel()
	f := newCheckpointFixture(t)
	f.admit(t, seedAt("bp-csv:2026-07-05", "health.blood-pressure", `{"systolic": 118, "diastolic": 76}`, time.Date(2026, 7, 5, 6, 30, 0, 0, time.UTC)))
	_, body := get(t, f.srv.URL+"/?view=whole")
	for _, want := range []string{"1 bucket, 0 sealed, ", "1 never minted", "no mint run yet", `class="bucket never"`} {
		if !strings.Contains(body, want) {
			t.Errorf("a never-minted ledger lacks %q\n%s", want, excerpt(body, "cp-summary"))
		}
	}
}

// excerpt returns the line of body holding marker, for a failure message.
func excerpt(body, marker string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, marker) {
			return line
		}
	}
	return "(no line holds " + marker + ")"
}
