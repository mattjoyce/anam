package query_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The equality check and the two questions run over a real ledger (AGENTS.md: real
// components over mocks): facts admitted through the gate, folded, served through a
// log-shaped projection, and only then handed to the evaluator.

// logTemplate is a log-shaped projection over every type the tests admit: what a
// query evaluates over. A snapshot-shaped one has already taken latest-per-series.
const logTemplate = `{
  "name": "everything", "version": 1, "description": "every type, as a log",
  "types": ["health.blood-pressure", "health.weight", "med.started", "med.dose-changed", "med.stopped"],
  "statuses": ["admitted", "endorsed"], "max_classification": "Sensitive",
  "from": "", "to": "", "shape": "log"
}`

type harness struct {
	l    *ledger.Ledger
	gate *gate.Gate
	defs *def.Registry
	tmpl projection.Template
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	l, err := ledger.Create(context.Background(), filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	defs := registry(t)
	scopes := gate.StaticScopes{
		"bp-csv":    {"health.blood-pressure"},
		"health-db": {"health.weight"},
		"self-log":  {"med.started", "med.dose-changed", "med.stopped"},
	}
	tmpl, err := projection.Parse([]byte(logTemplate))
	if err != nil {
		t.Fatalf("projection.Parse: %v", err)
	}
	return &harness{
		l: l, defs: defs, tmpl: tmpl,
		gate: gate.New(l, defs, scopes, clock.Real{}, "matt",
			gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries}),
	}
}

func (h *harness) admit(t *testing.T, source string, typ facet.Type, ref, payload string) {
	t.Helper()
	env := facet.Envelope{Type: typ, Source: source, Injector: source, SourceRef: facet.SourceRef(source, ref), Payload: []byte(payload)}
	binding, err := h.gate.Resolve(env)
	if err != nil {
		t.Fatalf("Resolve %s: %v", ref, err)
	}
	if _, _, err := h.gate.Commit(context.Background(), env, binding); err != nil {
		t.Fatalf("Commit %s: %v", ref, err)
	}
}

func (h *harness) med(t *testing.T, typ facet.Type, id, date, drug string, course int) {
	t.Helper()
	h.admit(t, "self-log", typ, id, fmt.Sprintf(`{"id": %q, "date": %q, "drug": %q, "course": %d}`, id, date, drug, course))
}

func (h *harness) bp(t *testing.T, measuredAt string, systolic, diastolic int) {
	t.Helper()
	h.admit(t, "bp-csv", "health.blood-pressure", measuredAt,
		fmt.Sprintf(`{"measured_at": %q, "systolic": %d, "diastolic": %d}`, measuredAt, systolic, diastolic))
}

// served is the log projection over the fold at the head, with the versions the
// base joins back to.
func (h *harness) served(t *testing.T) (fold.Value, map[string]int, fold.Result) {
	t.Helper()
	ctx := context.Background()
	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	res, err := fold.ReadAt(ctx, h.l, head)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	v, err := serve.Derive(res.Value, h.tmpl, consent.Open{}, h.defs, "")
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	return v, query.Versions(res.Live), res
}

// seedRegimen is the record the two 2026-10-17 questions are asked of: ramipril from
// March with a dose change in May; vitamin d3 + k2 from May, stopped mid-July,
// restarted as a second course in August; five readings across the year, one high.
func (h *harness) seedRegimen(t *testing.T) {
	t.Helper()
	h.med(t, "med.started", "m1", "2026-03-01", "ramipril", 1)
	h.med(t, "med.dose-changed", "m2", "2026-05-15", "ramipril", 1)
	h.med(t, "med.started", "m3", "2026-05-01", "vitamin d3 + k2", 1)
	h.med(t, "med.stopped", "m4", "2026-07-15", "vitamin d3 + k2", 1)
	h.med(t, "med.started", "m5", "2026-08-10", "vitamin d3 + k2", 2)
	h.bp(t, "2026-02-15T07:00:00Z", 121, 79)
	h.bp(t, "2026-04-10T07:00:00Z", 146, 92)
	h.bp(t, "2026-06-01T07:00:00Z", 118, 76)
	h.bp(t, "2026-07-20T07:00:00Z", 124, 80)
	h.bp(t, "2026-08-20T07:00:00Z", 117, 75)
}

// currentAt is the set of E with current(E, at) in the answer, sorted.
func currentAt(ans query.Answer, at string) []string {
	var ids []string
	for _, tup := range ans.Tuples {
		if tup[1].Text() == at {
			ids = append(ids, tup[0].Text())
		}
	}
	sort.Strings(ids)
	return ids
}

func snapshotIDs(t *testing.T, v fold.Value, at string, closes func(facet.Type) bool) []string {
	t.Helper()
	s, err := fold.Snapshot(v, at, closes)
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	var ids []string
	for _, e := range s.Entries() {
		ids = append(ids, e.FacetID)
	}
	sort.Strings(ids)
	return ids
}

// The equality check (card 173): the bundled current(E, At) equals fold.Snapshot at
// every fact-time the value holds, and at instants between and beyond them that the
// query names, over a log-shaped projection of a real ledger.
func TestEquality_CurrentIsTheSnapshot_RealLedger(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedRegimen(t)
	h.admit(t, "health-db", "health.weight", "2026-06-15T06:00:00Z", `{"measured_at": "2026-06-15T06:00:00Z", "kg": 81}`)
	v, versions, _ := h.served(t)
	if v.Len() != 11 {
		t.Fatalf("served %d entries, want 11", v.Len())
	}

	extra := []string{day("2025-12-31"), day("2026-05-01"), day("2026-07-15"), "2026-07-15T12:00:00.000000000Z", day("2027-01-01")}
	text := "instant(At) :- entry(_, _, At, _, _, _, _).\n"
	for _, at := range extra {
		text += fmt.Sprintf("instant(%q).\n", at)
	}
	ans, err := tryRun(v, versions, h.defs, text, "current")
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	var instants []string
	for _, e := range v.Entries() {
		instants = append(instants, e.FactTime)
	}
	instants = append(instants, extra...)
	for _, at := range instants {
		got, want := currentAt(ans, at), snapshotIDs(t, v, at, h.defs.Closes)
		if !same(got, want) {
			t.Errorf("current at %s = %v, Snapshot = %v", at, got, want)
		}
	}
	// Something was checked: the snapshot is non-trivial at the instants in between.
	if n := len(currentAt(ans, "2026-07-15T12:00:00.000000000Z")); n != 5 {
		t.Errorf("current mid-July has %d entries, want 5 (three readings, the dose change, the weight)", n)
	}
}

// The regimen at each blood-pressure reading, as one query: for each reading's
// instant, the drug of every regimen statement current then. The dose change in May
// carries ramipril forward; the stop in July removes vitamin d3 + k2 until the
// second course.
func TestQuestion_RegimenAtEachReading(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedRegimen(t)
	v, versions, _ := h.served(t)
	ans, err := tryRun(v, versions, h.defs, `
		instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
		regimen_type("med.started"). regimen_type("med.dose-changed").
		regimen(At, Drug) :- entry(_, "health.blood-pressure", At, _, _, _, _), current(E, At),
		                     entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", Drug).
	`, "regimen")
	if err != nil {
		t.Fatalf("regimen: %v", err)
	}
	want := []string{
		"2026-04-10T07:00:00.000000000Z ramipril",
		"2026-06-01T07:00:00.000000000Z ramipril",
		"2026-06-01T07:00:00.000000000Z vitamin d3 + k2",
		"2026-07-20T07:00:00.000000000Z ramipril",
		"2026-08-20T07:00:00.000000000Z ramipril",
		"2026-08-20T07:00:00.000000000Z vitamin d3 + k2",
	}
	if got := rows(ans); !same(got, want) {
		t.Errorf("regimen =\n%v\nwant\n%v", got, want)
	}
}

// The days without vitamin d3 + k2 in winter, as one query: the readings in June to
// August at whose instant no current regimen statement names the supplement. The
// payload is read where the question needs it (/drug, /systolic); the time is the
// envelope's.
func TestQuestion_DaysWithoutVitaminD3K2(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedRegimen(t)
	v, versions, _ := h.served(t)
	ans, err := tryRun(v, versions, h.defs, `
		instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
		regimen_type("med.started"). regimen_type("med.dose-changed").
		on_d3k2(At) :- current(E, At), entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", "vitamin d3 + k2").
		without(At) :- entry(_, "health.blood-pressure", At, _, _, _, _),
		               At >= "2026-06-01T00:00:00.000000000Z", At < "2026-09-01T00:00:00.000000000Z", not on_d3k2(At).
	`, "without")
	if err != nil {
		t.Fatalf("without: %v", err)
	}
	if got := rows(ans); !same(got, []string{"2026-07-20T07:00:00.000000000Z"}) {
		t.Errorf("without = %v, want the 20 July reading alone", got)
	}

	// And the selection by payload the doctrine now admits (§8): the high reading,
	// with what was current then.
	high, err := tryRun(v, versions, h.defs, `
		instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
		high(At, S, Drug) :- entry(R, "health.blood-pressure", At, _, _, _, _), attr(R, "/systolic", S), S > 140,
		                     current(E, At), entry(E, "med.started", _, _, _, _, _), attr(E, "/drug", Drug).
	`, "high")
	if err != nil {
		t.Fatalf("high: %v", err)
	}
	if got := rows(high); !same(got, []string{"2026-04-10T07:00:00.000000000Z 146 ramipril"}) {
		t.Errorf("high = %v", got)
	}
}

// Evaluation is a read of values: it takes no ledger, moves no watermark and writes
// nothing. The head and the canonical are the same before and after.
func TestEvaluate_MovesNoWatermark(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedRegimen(t)
	ctx := context.Background()
	v, versions, before := h.served(t)

	for i := 0; i < 2; i++ {
		ans, err := tryRun(v, versions, h.defs, `instant(At) :- entry(_, _, At, _, _, _, _).`, "current")
		if err != nil {
			t.Fatalf("current: %v", err)
		}
		if ans.Len() == 0 {
			t.Fatal("nothing was derived")
		}
	}

	head, err := h.l.Head(ctx)
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if head != before.W {
		t.Errorf("watermark moved from %d to %d", before.W, head)
	}
	after, err := fold.ReadAt(ctx, h.l, head)
	if err != nil {
		t.Fatalf("ReadAt: %v", err)
	}
	if after.Value.Hash() != before.Value.Hash() {
		t.Errorf("the canonical changed: %s → %s", before.Value.Hash(), after.Value.Hash())
	}
}
