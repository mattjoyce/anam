package inspect_test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The projection tests share one seeded ledger, built once for the package.
//
// AGENTS.md's rule is a unique table per test *run*, so nothing is shared across
// packages or between runs; within this package the four tests are strictly read-only
// over the same fixture, so a table each buys no isolation and costs four CreateTables
// against a DynamoDB Local that every other package is also hammering. That cost is not
// theoretical: four tables here is what pushed the package past the suite's timeout.
var (
	projOnce sync.Once
	projSrv  *httptest.Server
	projErr  error
)

func projectionServer(t *testing.T) *httptest.Server {
	t.Helper()
	if !reachable {
		t.Skipf("DynamoDB Local unreachable at %s", endpoint)
	}
	projOnce.Do(func() { projSrv, projErr = seedProjectionServer() })
	if projErr != nil {
		t.Fatalf("seed projection fixture: %v", projErr)
	}
	return projSrv
}

// seedProjectionServer builds the fixture: a facet the bp template serves, a live one
// it excludes by type, one that has been corrected (so a version is displaced), and one
// that has been tombstoned. The real ledger has no corrections in it, so this is the
// only place the superseded arm is exercised.
//
// Record-times are spread across two days so the as-of control has a boundary to land
// between. It returns an error rather than taking a *testing.T, because it runs inside
// a sync.Once where a t.Fatalf would report against whichever test happened to be
// first.
func seedProjectionServer() (*httptest.Server, error) {
	ctx := context.Background()

	l, err := ledger.New(ctx, endpoint, region, "anam_test_"+uuid.NewString(), person)
	if err != nil {
		return nil, fmt.Errorf("ledger.New: %w", err)
	}
	if err := l.Init(ctx); err != nil {
		return nil, fmt.Errorf("init: %w", err)
	}

	at := func(seq int64, day int) facet.RecordEvent {
		return facet.RecordEvent{RecordTime: time.Date(2026, 7, day, 9, 0, 0, 0, time.UTC), Seq: seq}
	}

	// seq 1 — a blood-pressure reading, admitted on the 10th.
	bp := seedFacetNoT("bp-csv:2026-01-24T06:30:00", "health.blood-pressure", `{"systolic": 118}`)
	ev := at(1, 10)
	ev.Kind, ev.FacetID, ev.ArtifactVersion = facet.EventAdmitted, bp.ID, 1
	if err := l.Admit(ctx, 0, bp, ev); err != nil {
		return nil, fmt.Errorf("admit bp: %w", err)
	}

	// seq 2 — a heart-rate facet, which bp excludes by type.
	hr := seedFacetNoT("health-db:hr:2026-07-05", "health.heart-rate", `{"resting": 54}`)
	ev = at(2, 10)
	ev.Kind, ev.FacetID, ev.ArtifactVersion = facet.EventAdmitted, hr.ID, 1
	if err := l.Admit(ctx, 1, hr, ev); err != nil {
		return nil, fmt.Errorf("admit hr: %w", err)
	}

	// seq 3 — the reading is corrected on the 12th: v1 is displaced by v2.
	corrected := seedFacetNoT("bp-csv:2026-01-24T06:30:00", "health.blood-pressure", `{"systolic": 117}`)
	corrected.ArtifactVersion = 2
	ev = at(3, 12)
	ev.Kind, ev.FacetID, ev.ArtifactVersion, ev.PriorArtifactVersion = facet.EventSuperseded, corrected.ID, 2, 1
	if err := l.Admit(ctx, 2, corrected, ev); err != nil {
		return nil, fmt.Errorf("admit correction: %w", err)
	}

	// seq 4 — the heart-rate facet is retracted on the 12th.
	ev = at(4, 12)
	ev.Kind, ev.FacetID, ev.ArtifactVersion = facet.EventTombstoned, hr.ID, 1
	if err := l.Mark(ctx, 3, ev); err != nil {
		return nil, fmt.Errorf("mark tombstone: %w", err)
	}

	// seq 5 — a sleep period, admitted on the 12th and left live. A *live* facet of
	// another type is what exercises the template's type exclusion: the tombstoned
	// heart-rate above never reaches a template at all, because the canonical is
	// live-only by construction.
	sleep := seedFacetNoT("health-db:sleep:2026-07-05", "health.sleep", `{"deep_minutes": 62}`)
	ev = at(5, 12)
	ev.Kind, ev.FacetID, ev.ArtifactVersion = facet.EventAdmitted, sleep.ID, 1
	if err := l.Admit(ctx, 4, sleep, ev); err != nil {
		return nil, fmt.Errorf("admit sleep: %w", err)
	}

	// A generous per-request budget: the page is several ledger round-trips, and the
	// whole suite shares one DynamoDB Local. Latency is not what these tests are about.
	return httptest.NewServer(inspect.Handler(l, 90*time.Second)), nil
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
	for _, want := range []string{"anam projections", "canonical", "bp", "sleep", "as-of"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not mention %q", want)
		}
	}
	// One live bp (at v2), one tombstoned hr; displaced v1 and the dead hr are the
	// two superseded rows.
	if !strings.Contains(body, "superseded") {
		t.Error("no superseded rows rendered")
	}
	if !strings.Contains(body, "displaced by v2") {
		t.Errorf("the displaced version is not explained:\n%s", body)
	}
	if !strings.Contains(body, "not live: tombstoned") {
		t.Error("the tombstoned facet is not explained")
	}
}

// Switching the template must change the verdict on every facet, and say why.
func TestProjectionPage_SwitchingTemplateChangesTheVerdicts(t *testing.T) {
	t.Parallel()
	srv := projectionServer(t)

	code, bp := get(t, srv.URL+"/projection?p=bp")
	if code != 200 {
		t.Fatalf("GET /projection?p=bp = %d", code)
	}
	// The live sleep facet is what the type knob excludes here. The tombstoned
	// heart-rate never reaches the template at all — the canonical is live-only by
	// construction, so a dead facet is a superseded row, not an exclusion.
	if !strings.Contains(bp, "type not in the allow-list") {
		t.Errorf("bp does not explain why the live sleep facet is out:\n%s", bp)
	}
	if !strings.Contains(bp, "health.blood-pressure") {
		t.Error("bp does not show the reading it serves")
	}

	// The same ledger through the other template: the verdicts swap over. That swap
	// IS the demonstration of governance — one read, two governed variants, and the
	// difference is visible without re-running anything.
	code, sleepBody := get(t, srv.URL+"/projection?p=sleep")
	if code != 200 {
		t.Fatalf("GET /projection?p=sleep = %d", code)
	}
	if !strings.Contains(sleepBody, "type not in the allow-list") {
		t.Error("sleep does not explain its exclusions")
	}
	if bp == sleepBody {
		t.Error("switching the template changed nothing on the page")
	}

	// The served hashes must differ: they are different values, so they are
	// different content addresses.
	if servedHash(t, bp) == servedHash(t, sleepBody) {
		t.Error("bp and sleep served the same hash over a ledger holding both types")
	}

	// And the hash is identified as the receipt's — that is what makes the viewer
	// checkable against the publisher rather than merely plausible.
	if !strings.Contains(bp, "a receipt would carry") {
		t.Error("the served hash is not identified as the receipt's")
	}
	if !strings.Contains(bp, "bp v1") {
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
	code, before := get(t, srv.URL+"/projection?p=bp&asof=2026-07-11T00:00:00Z")
	if code != 200 {
		t.Fatalf("as-of GET = %d\n%s", code, before)
	}
	if !strings.Contains(before, ">2</strong> watermark") && !strings.Contains(before, "<strong>2</strong>") {
		t.Errorf("as-of the 11th should read W=2:\n%s", before)
	}
	if strings.Contains(before, "displaced by v2") {
		t.Error("as-of the 11th, nothing had been displaced yet")
	}
	if strings.Contains(before, "not live: tombstoned") {
		t.Error("as-of the 11th, nothing had been tombstoned yet")
	}

	// Now: both have happened.
	_, now := get(t, srv.URL+"/projection?p=bp")
	if !strings.Contains(now, "displaced by v2") || !strings.Contains(now, "not live: tombstoned") {
		t.Error("the head view lost the supersession or the tombstone")
	}

	// And before anything was admitted, the honest answer is an empty projection
	// rather than an error — a receipt could legitimately name that watermark.
	code, genesis := get(t, srv.URL+"/projection?p=bp&asof=2026-01-01T00:00:00Z")
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

	if code, _ := get(t, srv.URL+"/projection?p=bloodpressure"); code != 404 {
		t.Errorf("unknown projection = %d, want 404", code)
	}
	if code, _ := get(t, srv.URL+"/projection?asof=last-tuesday"); code != 400 {
		t.Errorf("unparseable as-of = %d, want 400", code)
	}
}
