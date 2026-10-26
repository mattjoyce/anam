package query_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/query"
)

const (
	noon     = "2026-06-15T12:00:00.000000000Z"
	sixLater = "2026-06-15T18:00:00.000000000Z"
)

// within(A, B, Days) is true when two fact-times are at most Days days apart, a day
// being 24 hours of UTC. The pairs are facts, so every argument is bound from data,
// which is the path a real query takes; the bad rows are data the builtin must call
// false, not refuse.
func TestWithin_Table(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		a, b string
		days string
		want bool
	}{
		{"same instant, zero days", noon, noon, "0", true},
		{"one nanosecond apart, zero days", noon, "2026-06-15T12:00:00.000000001Z", "0", false},
		{"six hours is a quarter day", noon, sixLater, "0.25", true},
		{"and the other way round", sixLater, noon, "0.25", true},
		{"a nanosecond past a quarter day", noon, "2026-06-15T18:00:00.000000001Z", "0.25", false},
		{"a nanosecond short of a quarter day", noon, "2026-06-15T17:59:59.999999999Z", "0.25", true},
		{"one day", noon, "2026-06-16T12:00:00.000000000Z", "1", true},
		{"a second over one day", noon, "2026-06-16T12:00:01.000000000Z", "1", false},
		{"fifty-six days across a month end", "2026-05-01T07:00:00.000000000Z", "2026-06-26T07:00:00.000000000Z", "56", true},
		{"fifty-six days and a nanosecond", "2026-05-01T07:00:00.000000000Z", "2026-06-26T07:00:00.000000001Z", "56", false},
		{"exponent form", noon, "2026-06-16T12:00:00.000000000Z", "1e0", true},
		{"a tiny fraction of a day, one microsecond", noon, "2026-06-15T12:00:00.000001000Z", "0.0000000000115740740740", false},
		{"enough days to cover the whole range of the clock", "0001-01-01T00:00:00.000000000Z", "9999-12-31T23:59:59.999999999Z", "3700000", true},
		{"a little short of the whole range", "0001-01-01T00:00:00.000000000Z", "9999-12-31T23:59:59.999999999Z", "3600000", false},
		{"an enormous exponent covers anything", "0001-01-01T00:00:00.000000000Z", "9999-12-31T23:59:59.999999999Z", "1e999999999", true},
		{"a vanishing exponent covers only the same instant", noon, "2026-06-15T12:00:00.000000001Z", "1e-999999999", false},
		{"a vanishing exponent, same instant", noon, noon, "1e-999999999", true},

		// Data that is not what the builtin takes is false, not an error: it came
		// from a record, and the program was right to ask.
		{"a date is not a fact-time", "2026-06-15", noon, "1", false},
		{"nor is a sloppy instant", "2026-06-15T12:00:00Z", noon, "1", false},
		{"nor a zoned one", "2026-06-15T22:00:00.000000000+10:00", noon, "1", false},
		{"B is checked too", noon, "yesterday", "1", false},
		{"negative days from data", noon, noon, "-1", false},
		{"negative zero is zero", noon, noon, "-0", true},
	}
	var pairs strings.Builder
	for i, tt := range tests {
		fmt.Fprintf(&pairs, "pair(%d, %q, %q, %s).\n", i, tt.a, tt.b, tt.days)
	}
	v, versions := value(t)
	ans, err := tryRun(v, versions, registry(t), pairs.String()+`
		holds(I) :- pair(I, A, B, D), within(A, B, D).
	`, "holds")
	if err != nil {
		t.Fatalf("holds: %v", err)
	}
	got := map[string]bool{}
	for _, r := range rows(ans) {
		got[r] = true
	}
	for i, tt := range tests {
		if has := got[fmt.Sprint(i)]; has != tt.want {
			t.Errorf("%s: within(%s, %s, %s) = %v, want %v", tt.name, tt.a, tt.b, tt.days, has, tt.want)
		}
	}
}

// A Days that is the wrong kind from data is false, not a crash.
func TestWithin_DaysFromDataOfTheWrongKind(t *testing.T) {
	t.Parallel()
	v, versions := value(t)
	ans, err := tryRun(v, versions, registry(t), fmt.Sprintf(`
		pair("one", %q, %q, "1").
		pair("two", %q, %q, true).
		holds(I) :- pair(I, A, B, D), within(A, B, D).
	`, noon, noon, noon, noon), "holds")
	if err != nil {
		t.Fatalf("holds: %v", err)
	}
	if ans.Len() != 0 {
		t.Errorf("a string or a boolean for Days held: %v", rows(ans))
	}
}

// not within(...) is the complement over bound values: the pairs more than Days apart.
func TestWithin_Negated(t *testing.T) {
	t.Parallel()
	v, versions := value(t)
	ans, err := tryRun(v, versions, registry(t), fmt.Sprintf(`
		pair("near", %q, %q).
		pair("far", %q, %q).
		apart(I) :- pair(I, A, B), not within(A, B, 0.25).
	`, noon, sixLater, noon, "2026-06-16T12:00:00.000000000Z"), "apart")
	if err != nil {
		t.Fatalf("apart: %v", err)
	}
	if got := rows(ans); !same(got, []string{"far"}) {
		t.Errorf("apart = %v, want far alone", got)
	}
}

// A builtin is a guard, not a relation: it needs no declaration, defines nothing,
// and does not enter the stratification, so a recursive rule may carry one.
func TestWithin_InARecursiveRule(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("a", "health.weight", "2026-06-01T00:00:00.000000000Z", "", `{}`),
		entry("b", "health.weight", "2026-06-03T00:00:00.000000000Z", "", `{}`),
		entry("c", "health.weight", "2026-06-05T00:00:00.000000000Z", "", `{}`),
		entry("d", "health.weight", "2026-06-20T00:00:00.000000000Z", "", `{}`),
	)
	// A chain of readings each within two days of the next: a, b, c; d stands alone.
	ans, err := tryRun(v, versions, registry(t), `
		step(E, F) :- entry(E, _, A, _, _, _, _), entry(F, _, B, _, _, _, _), A < B, within(A, B, 2).
		chain(E, F) :- step(E, F).
		chain(E, G) :- chain(E, F), step(F, G).
	`, "chain")
	if err != nil {
		t.Fatalf("chain: %v", err)
	}
	if got := rows(ans); !same(got, []string{"a b", "a c", "b c"}) {
		t.Errorf("chain = %v", got)
	}
}

// The plan check refuses a builtin it cannot evaluate, naming the predicate and the
// argument, as it refuses an unbound comparison.
func TestWithin_RefusedAtPlanTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, text string
		want       []string
	}{
		{"Days unbound", `r(E) :- entry(E, _, A, _, _, _, _), within(A, A, D).`, []string{"within", "D", "must be bound by a positive body atom"}},
		{"A unbound", `r(E) :- entry(E, _, B, _, _, _, _), within(X, B, 1).`, []string{"within", "X"}},
		{"B anonymous", `r(E) :- entry(E, _, A, _, _, _, _), within(A, _, 1).`, []string{"within", "_"}},
		{"bound only by a negation", `r(E) :- entry(E, _, A, _, _, _, _), not entry(_, _, A, _, _, _, _), within(A, A, D).`, []string{"within", "D"}},
		{"bound only by another builtin", `r(E) :- entry(E, _, A, _, _, _, _), within(A, A, 1), within(A, B, 1).`, []string{"within", "B"}},
		{"negated and unbound", `r(E) :- entry(E, _, A, _, _, _, _), not within(A, B, 1).`, []string{"within", "B"}},
		{"too few arguments", `r(E) :- entry(E, _, A, _, _, _, _), within(A, 1).`, []string{"within has 2 arguments", "within/3"}},
		{"too many arguments", `r(E) :- entry(E, _, A, _, _, _, _), within(A, A, 1, 2).`, []string{"within has 4 arguments", "within/3"}},
		{"a bare date", `r(E) :- entry(E, _, A, _, _, _, _), within("2026-06-15", A, 1).`, []string{"within", "argument A", "not a fact-time", "2026-06-15T00:00:00.000000000Z"}},
		{"a shortened instant", `r(E) :- entry(E, _, A, _, _, _, _), within(A, "2026-06-15T12:00:00Z", 1).`, []string{"argument B", "not a fact-time"}},
		{"a number for A", `r(E) :- entry(E, _, A, _, _, _, _), within(7, A, 1).`, []string{"argument A", "not a fact-time"}},
		{"negative days", `r(E) :- entry(E, _, A, _, _, _, _), within(A, A, -1).`, []string{"argument Days", "negative"}},
		{"days as a string", `r(E) :- entry(E, _, A, _, _, _, _), within(A, A, "1").`, []string{"argument Days", "not a number"}},
		{"a rule that defines it", `within(A, B, D) :- entry(A, _, B, _, _, _, _), entry(D, _, _, _, _, _, _).`, []string{"within is a built-in", "head of a rule"}},
		{"a declaration of it", "within/3.\nr(1).", []string{"within is a built-in", "cannot be declared"}},
	}
	v, versions := value(t, entry("w", "health.weight", noon, "", `{}`))
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := tryRun(v, versions, registry(t), tt.text, "r")
			if !errors.Is(err, query.ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("the refusal %q does not say %q", err, w)
				}
			}
		})
	}
}

// A constant instant of the right shape is accepted beside variables.
func TestWithin_ConstantsAreAcceptedWhenRight(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("a", "health.weight", "2026-06-15T10:00:00.000000000Z", "", `{}`),
		entry("b", "health.weight", "2026-07-15T10:00:00.000000000Z", "", `{}`),
	)
	ans, err := tryRun(v, versions, registry(t), fmt.Sprintf(`
		near(E) :- entry(E, _, T, _, _, _, _), within(T, %q, 0.5).
	`, noon), "near")
	if err != nil {
		t.Fatalf("near: %v", err)
	}
	if got := rows(ans); !same(got, []string{"a"}) {
		t.Errorf("near = %v, want a", got)
	}
}

// within against a brute-force computation over every pair of instants in a real
// ledger's fixture, for a spread of Days. The Soufflé oracle covers the rules and
// cannot express this builtin (scripts/souffle-diff.py), so this is what holds it.
func TestWithin_AgainstBruteForce_RealLedger(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedRegimen(t)
	h.admit(t, "health-db", "health.weight", "2026-06-15T06:00:00Z", `{"measured_at": "2026-06-15T06:00:00Z", "kg": 81}`)
	h.admit(t, "health-db", "health.weight", "2026-06-15T12:00:00Z", `{"measured_at": "2026-06-15T12:00:00Z", "kg": 80}`)
	h.admit(t, "health-db", "health.weight", "2026-06-15T12:00:00.5Z", `{"measured_at": "2026-06-15T12:00:00.5Z", "kg": 80}`)
	v, versions, _ := h.served(t)

	var instants []time.Time
	seen := map[string]bool{}
	for _, e := range v.Entries() {
		if seen[e.FactTime] {
			continue
		}
		seen[e.FactTime] = true
		at, err := clock.Parse(e.FactTime)
		if err != nil {
			t.Fatal(err)
		}
		instants = append(instants, at)
	}
	if len(instants) < 10 {
		t.Fatalf("the fixture has %d distinct instants, want at least 10", len(instants))
	}

	const day = 24 * time.Hour
	spread := []struct {
		days string
		span time.Duration
	}{
		{"0", 0},
		{"0.5", day / 2},
		{"0.25", day / 4},
		{"1", day},
		{"1.5", day + day/2},
		{"7", 7 * day},
		{"30", 30 * day},
		{"56", 56 * day},
		{"90.125", 90*day + day/8},
		{"400", 400 * day},
	}
	for _, s := range spread {
		ans, err := tryRun(v, versions, h.defs, fmt.Sprintf(`
			near(A, B) :- entry(_, _, A, _, _, _, _), entry(_, _, B, _, _, _, _), within(A, B, %s).
		`, s.days), "near")
		if err != nil {
			t.Fatalf("days %s: %v", s.days, err)
		}
		got := map[string]bool{}
		for _, r := range rows(ans) {
			got[r] = true
		}
		var want, checked int
		for _, a := range instants {
			for _, b := range instants {
				diff := a.Sub(b)
				if diff < 0 {
					diff = -diff
				}
				expect := diff <= s.span
				key := clock.Format(a) + " " + clock.Format(b)
				if got[key] != expect {
					t.Errorf("days %s: within(%s, %s) = %v, brute force says %v (apart %v)", s.days, clock.Format(a), clock.Format(b), got[key], expect, diff)
				}
				checked++
				if expect {
					want++
				}
			}
		}
		if len(got) != want {
			t.Errorf("days %s: %d pairs held, brute force counts %d", s.days, len(got), want)
		}
		if s.days == "56" && (want == 0 || want == checked) {
			t.Errorf("days 56 held %d of %d pairs: the check discriminates nothing", want, checked)
		}
	}
}

// The catalogue lists the builtins with their contracts, and says what a day is.
func TestDescribe_ListsTheBuiltins(t *testing.T) {
	t.Parallel()
	cat, err := query.Describe(registry(t), nil)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(cat.Builtins) == 0 || cat.Builtins[0].Name != "within" {
		t.Fatalf("builtins = %+v, want within first", cat.Builtins)
	}
	b := cat.Builtins[0]
	if len(b.Args) != 3 || b.Args[0].Name != "A" || b.Args[1].Name != "B" || b.Args[2].Name != "Days" {
		t.Errorf("within's arguments = %+v", b.Args)
	}
	for _, a := range b.Args {
		if !a.Bound {
			t.Errorf("argument %s is not required bound", a.Name)
		}
	}
	for _, w := range []string{"24 hours of UTC", "not negative", "0.25"} {
		if !strings.Contains(b.Doc, w) {
			t.Errorf("within's contract does not say %q: %s", w, b.Doc)
		}
	}
	if !strings.Contains(cat.Syntax, "24 hours of UTC") {
		t.Errorf("the syntax paragraph does not say what a day is")
	}
}
