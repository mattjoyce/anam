package query_test

import (
	"testing"
)

// A number in a payload beyond float64's exact range is silently changed by the
// engine. The record's lexeme is exact, since def.Decode reads numbers as json.Number,
// but Const keeps one canonical lexeme per number, and the lexeme it keeps is
// the float64's (internal/query/term.go:38). Two distinct record numbers
// therefore become one constant, and a comparison answers with the wrong fact.

// 2^53+1: the first integer a float64 cannot hold.
const beyondFloat64 = "9007199254740993"

func TestBug_AttrReportsTheNumberTheRecordHolds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-06-15T07:00:00Z",
		`{"measured_at": "2026-06-15T07:00:00Z", "systolic": 9007199254740993, "diastolic": 80}`)
	v, versions, _ := h.served(t)

	ans, err := tryRun(v, versions, h.defs,
		`sys(E, S) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/systolic", S).`, "sys")
	if err != nil {
		t.Fatalf("attr: %v", err)
	}
	if ans.Len() != 1 {
		t.Fatalf("attr bound %d times, want 1", ans.Len())
	}
	if got := ans.Tuples[0][1].Text(); got != beyondFloat64 {
		t.Errorf("attr reported %s, want the number the record holds, %s", got, beyondFloat64)
	}
}

func TestBug_ComparingANumberBeyondFloat64DoesNotMatchTheWrongFact(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	// 2^53: the largest integer a float64 holds exactly, and the neighbour of the
	// number the query asks for.
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-06-15T07:00:00Z",
		`{"measured_at": "2026-06-15T07:00:00Z", "systolic": 9007199254740992, "diastolic": 80}`)
	v, versions, _ := h.served(t)

	ans, err := tryRun(v, versions, h.defs,
		`sys(E) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/systolic", S), S = `+beyondFloat64+`.`, "sys")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ans.Len() != 0 {
		t.Errorf("the query answered %d rows; no fact holds %s; the fact it matched holds 9007199254740992, "+
			"which is a different number", ans.Len(), beyondFloat64)
	}
}
