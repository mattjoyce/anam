package query_test

import (
	"errors"
	"sort"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
)

// in_force(E, F) is current(E, At) at the fact-time of F, for the F whose fact-time
// the query names in instant/1 and for no other. Held equal to current/2 over the
// blood-pressure readings' instants, and to fold.Snapshot there, on a real ledger.
func TestInForce_IsCurrentAtTheReadingsInstants(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedRegimen(t)
	h.admit(t, "health-db", "health.weight", "2026-06-15T06:00:00Z", `{"measured_at": "2026-06-15T06:00:00Z", "kg": 81}`)
	v, versions, _ := h.served(t)

	const instants = `instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).`
	inForce, err := tryRun(v, versions, h.defs, instants, "in_force")
	if err != nil {
		t.Fatalf("in_force: %v", err)
	}
	current, err := tryRun(v, versions, h.defs, instants, "current")
	if err != nil {
		t.Fatalf("current: %v", err)
	}

	byReading := map[string][]string{}
	for _, tup := range inForce.Tuples {
		byReading[tup[1].Text()] = append(byReading[tup[1].Text()], tup[0].Text())
	}
	var readings int
	august := ""
	for _, e := range v.Entries() {
		if e.FactTime == "2026-08-20T07:00:00.000000000Z" {
			august = e.FacetID
		}
		if e.Type != "health.blood-pressure" {
			if _, ok := byReading[e.FacetID]; ok {
				t.Errorf("in_force holds for %s, a %s whose instant the query did not name", e.FacetID, e.Type)
			}
			continue
		}
		readings++
		got := byReading[e.FacetID]
		sort.Strings(got)
		if want := currentAt(current, e.FactTime); !same(got, want) {
			t.Errorf("in_force at reading %s (%s) = %v, current there = %v", e.FacetID, e.FactTime, got, want)
		}
		if want := snapshotIDs(t, v, e.FactTime, h.defs.Closes); !same(got, want) {
			t.Errorf("in_force at reading %s = %v, Snapshot = %v", e.FacetID, got, want)
		}
	}
	if readings != 5 {
		t.Fatalf("%d readings, want 5", readings)
	}
	// Something was compared: the mid-year readings stand on a non-trivial record.
	if n := len(byReading[august]); n < 5 {
		t.Errorf("the August reading has %d statements in force, want at least 5", n)
	}
}

// Without instant/1 defined, in_force has nothing to ask about, and the plan check
// says so as it does for current/2.
func TestInForce_NeedsTheInstants(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedRegimen(t)
	v, versions, _ := h.served(t)
	_, err := tryRun(v, versions, h.defs, `held(E) :- in_force(E, _).`, "held")
	if err == nil {
		t.Fatal("in_force without instants ran")
	}
	if !errors.Is(err, query.ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid", err)
	}
}
