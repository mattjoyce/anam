package query_test

import (
	"fmt"
	"sort"
	"testing"
)

// A number in a payload is the exact decimal the record holds. The engine kept a
// float64 and printed the float's shortest form, so 1792269840000 answered as
// 1.79226984e+12, 9007199254740993 answered as the same number as its neighbour
// 9007199254740992, and a comparison with the wrong number matched. The value is
// now exact: it answers as the record holds it, an integer in full, and it
// compares exactly.

func TestNumber_TheValueTheRecordHoldsIsTheValueThatAnswers(t *testing.T) {
	t.Parallel()
	cases := []struct {
		lexeme string
		want   string
	}{
		{"120", "120"},                           // an ordinary reading
		{"81.4", "81.4"},                         // a decimal
		{"1792269840000", "1792269840000"},       // an epoch in milliseconds, in full
		{"9007199254740993", "9007199254740993"}, // beyond float64's exact range
		{"1.0000000000000000001", "1.0000000000000000001"},
		{"0.5", "0.5"},
		{"-0.5", "-0.5"},
		{"1.0", "1"},    // one value, one form
		{"1e3", "1000"}, // the same value as 1000
	}
	h := newHarness(t)
	for i, c := range cases {
		at := fmt.Sprintf("2026-06-%02dT07:00:00Z", i+1)
		h.admit(t, "bp-csv", "health.blood-pressure", at,
			fmt.Sprintf(`{"measured_at": %q, "systolic": %s}`, at, c.lexeme))
	}
	v, versions, _ := h.served(t)

	ans, err := tryRun(v, versions, h.defs,
		`sys(E, S) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/systolic", S).`, "sys")
	if err != nil {
		t.Fatalf("attr: %v", err)
	}
	var got []string
	for _, tup := range ans.Tuples {
		got = append(got, tup[1].Text())
	}
	sort.Strings(got)
	var want []string
	for _, c := range cases {
		want = append(want, c.want)
	}
	sort.Strings(want)
	if !same(got, want) {
		t.Errorf("the numbers answered\n%v\nwant\n%v", got, want)
	}
}

// Two numbers a float64 cannot tell apart are two numbers: 1 and
// 1.0000000000000000001 are the same float64, and they are not the same decimal.
func TestNumber_TwoNumbersAFloat64CannotTellApartAreNotOne(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-06-15T07:00:00Z",
		`{"measured_at": "2026-06-15T07:00:00Z", "systolic": 1.0000000000000000001}`)
	v, versions, _ := h.served(t)

	ans, err := tryRun(v, versions, h.defs,
		`sys(E) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/systolic", S), S = 1.`, "sys")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ans.Len() != 0 {
		t.Errorf("the query answered %d rows; the fact holds 1.0000000000000000001, which is not 1", ans.Len())
	}
}

// The number the record holds is the number a comparison matches: a fact holding
// 9007199254740993 answers a query that asks for it.
func TestNumber_TheNumberTheRecordHoldsIsTheNumberAComparisonMatches(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-06-15T07:00:00Z",
		`{"measured_at": "2026-06-15T07:00:00Z", "systolic": 9007199254740993}`)
	v, versions, _ := h.served(t)

	ans, err := tryRun(v, versions, h.defs,
		`sys(E) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/systolic", S), S = 9007199254740993.`, "sys")
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if ans.Len() != 1 {
		t.Errorf("the query answered %d rows, want 1", ans.Len())
	}
}
