package query_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/query"
)

// lazyFixture is a served value with readings, notes and an edge between them, so
// every program in the table has something to find.
func lazyFixture(t *testing.T) (*harness, fold.Value, map[string]int) {
	t.Helper()
	h := edgeHarness(t)
	h.bp(t, "2026-02-15T07:00:00Z", 121, 79)
	h.bp(t, "2026-04-10T07:00:00Z", 146, 92)
	v, _, _ := h.served(t)
	var reading string
	for _, e := range v.Entries() {
		reading = e.FacetID
	}
	h.admit(t, "records", "record.edge", "note:1", `{"id": "note:1", "at": "2026-04-10 18:00", "source_time": {"value": "2026-04-10 18:00", "zone": "Australia/Sydney", "offset": "+10:00", "basis": "source"}, "about": ["`+reading+`"], "text": "Before the dentist."}`)
	h.admit(t, "records", "record.edge", "interp:1", `{"id": "interp:1", "at": "2026-04-11 09:00", "source_time": {"value": "2026-04-11 09:00", "zone": "Australia/Sydney", "offset": "+10:00", "basis": "source"}, "kind": "interpreted_from", "from": ["`+reading+`"], "to": ["`+reading+`"]}`)
	v, versions, _ := h.served(t)
	return h, v, versions
}

// answerWith evaluates text over the fixture with attr/3 built or not, as the caller
// says, and returns the named relation.
func answerWith(t *testing.T, h *harness, v fold.Value, versions map[string]int, text, relation string, withAttr bool) (query.Answer, query.Base) {
	t.Helper()
	rules, err := query.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	prog, err := query.Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	base, err := query.NewBase(v, versions, h.defs, withAttr)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	plan, err := query.Compile(query.BaseSchema, h.defs, rules.Program, prog)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	res, err := query.Evaluate(context.Background(), plan, base, 0)
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	ans, err := res.Relation(relation)
	if err != nil {
		t.Fatalf("Relation(%s): %v", relation, err)
	}
	return ans, base
}

// The decision, and the answer it must not change: for each program the answer with
// attr/3 built only when ReadsAttr says so equals the answer with it always built,
// and the base holds attr tuples exactly when the decision was to build them.
func TestReadsAttr_LazyAnswersEqualEagerOnes(t *testing.T) {
	t.Parallel()
	h, v, versions := lazyFixture(t)
	cases := []struct {
		name, text, relation string
		readsAttr            bool
	}{
		{"entry only", `bp(E, At) :- entry(E, "health.blood-pressure", At, _, _, _, _).`, "bp", false},
		{"closes", `c(T) :- closes(T).`, "c", false},
		{"bundled current, no attr", `instant(At) :- entry(_, _, At, _, _, _, _). now(E) :- current(E, At).`, "now", false},
		{"attr directly", `high(E, V) :- attr(E, "/systolic", V), V > 130.`, "high", true},
		{"attr by a variable pointer", `leaf(E, P, V) :- attr(E, P, V).`, "leaf", true},
		{"attr through linked", `touched(E) :- linked(E, _).`, "touched", true},
		{"attr through edge_to", `into(E, F) :- edge_to(E, F).`, "into", true},
		{"attr through a rule chain", `a(E) :- linked(E, _). b(E) :- a(E). c(E) :- entry(E, _, _, _, _, _, _), not b(E).`, "c", true},
		{"a program with attr that the root does not reach", `unused(E) :- attr(E, "/systolic", _). bp(E) :- entry(E, "health.blood-pressure", _, _, _, _, _).`, "bp", false},
		{"linked asked for itself", ``, "linked", true},
		{"attr asked for itself", ``, "attr", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			rules, err := query.Bundled()
			if err != nil {
				t.Fatal(err)
			}
			prog, err := query.Parse(c.text)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			lazy := query.ReadsAttr([]string{c.relation}, rules.Program, prog)
			if lazy != c.readsAttr {
				t.Fatalf("ReadsAttr = %v, want %v", lazy, c.readsAttr)
			}
			got, base := answerWith(t, h, v, versions, c.text, c.relation, lazy)
			want, eager := answerWith(t, h, v, versions, c.text, c.relation, true)
			if got.Len() == 0 {
				t.Fatalf("%s is empty; the fixture should give it rows", c.relation)
			}
			if fmt.Sprint(rows(got)) != fmt.Sprint(rows(want)) {
				t.Errorf("lazy answer = %v, eager answer = %v", rows(got), rows(want))
			}
			if built := base.Len(query.RelAttr) > 0; built != c.readsAttr {
				t.Errorf("attr tuples built = %v, want %v", built, c.readsAttr)
			}
			if eager.Len(query.RelAttr) == 0 {
				t.Error("the fixture has no attr tuples to leave out")
			}
		})
	}
}

// An entry-only program over a fixture builds no attr tuple at all, and the same
// served value with attr wanted builds them.
func TestNewBase_EntryOnlyBuildsNoAttr(t *testing.T) {
	t.Parallel()
	h, v, versions := lazyFixture(t)
	base, err := query.NewBase(v, versions, h.defs, false)
	if err != nil {
		t.Fatal(err)
	}
	if n := base.Len(query.RelAttr); n != 0 {
		t.Errorf("attr tuples = %d, want 0", n)
	}
	if n, want := base.Len(query.RelEntry), len(v.Entries()); n != want {
		t.Errorf("entry tuples = %d, want %d", n, want)
	}
	full, err := query.NewBase(v, versions, h.defs, true)
	if err != nil {
		t.Fatal(err)
	}
	if full.Len(query.RelAttr) == 0 {
		t.Error("attr tuples = 0 with attr wanted")
	}
	if full.Len(query.RelEntry) != base.Len(query.RelEntry) || full.Len(query.RelCloses) != base.Len(query.RelCloses) {
		t.Error("entry/7 and closes/1 differ between the two builds")
	}
}

// ReadsAttr follows negation, and a root no program defines reaches nothing.
func TestReadsAttr_WalksTheRules(t *testing.T) {
	t.Parallel()
	rules, err := query.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		text, root string
		want       bool
	}{
		{`a(E) :- entry(E, _, _, _, _, _, _), not x(E). x(E) :- attr(E, _, _).`, "a", true},
		{`a(E) :- entry(E, _, _, _, _, _, _).`, "current", false},
		{`a(E) :- entry(E, _, _, _, _, _, _).`, "nothing", false},
		{`r(E) :- r(E), linked(E, _). r(E) :- entry(E, _, _, _, _, _, _).`, "r", true},
		{`r(E) :- r(E). r(E) :- entry(E, _, _, _, _, _, _).`, "r", false},
	} {
		prog, err := query.Parse(c.text)
		if err != nil {
			t.Fatalf("%s: %v", c.text, err)
		}
		if got := query.ReadsAttr([]string{c.root}, rules.Program, prog); got != c.want {
			t.Errorf("%s from %s: ReadsAttr = %v, want %v", strings.TrimSpace(c.text), c.root, got, c.want)
		}
	}
}
