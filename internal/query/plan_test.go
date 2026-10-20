package query_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
)

// Validation refuses what cannot run, before anything is read, with a message that
// says what to change (card 173: errors that teach the agent).
func TestCompile_RefusesWithAReason(t *testing.T) {
	t.Parallel()
	defs := registry(t)
	rules, err := query.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	cases := []struct {
		name, text, want string
	}{
		{"unknown relation", `a(X) :- entry(X, _, _, _, _, _, _), nothing(X).`, "unknown relation nothing/1; the relations are a/1, attr/3, before/2"},
		{"arity", `a(X) :- entry(X, _, _).`, "entry has 3 arguments, but entry is entry/7"},
		{"two arities for one head", `a(X) :- entry(X, _, _, _, _, _, _). a(X, Y) :- entry(X, Y, _, _, _, _, _).`, "a/2, but a is a/1"},
		{"rule over a base relation", `entry(X, "t", "", "", "", "", 1) :- attr(X, _, _).`, "entry/7 is a base relation; a rule cannot add to it"},
		{"declaring a base relation", `attr/3.`, "attr/3 is a base relation and cannot be declared"},
		{"unbound head variable", `a(X, Y) :- entry(X, _, _, _, _, _, _).`, "Y in the head of a is not bound by a positive body atom"},
		{"anonymous head", `a(_) :- entry(_, _, _, _, _, _, _).`, "_ in the head of a"},
		{"unbound in negation", `a(X) :- entry(X, _, _, _, _, _, _), not attr(X, P, _), P != "".`, "P in not attr is not bound"},
		{"unbound in comparison", `a(X) :- entry(X, _, _, _, _, _, _), Y > 1.`, "Y in a comparison is not bound"},
		{"anonymous in comparison", `a(X) :- entry(X, _, _, _, _, _, _), _ > 1.`, "_ in a comparison compares nothing"},
		{"unstratifiable", `a(X) :- entry(X, _, _, _, _, _, _), not b(X). b(X) :- entry(X, _, _, _, _, _, _), not a(X).`, "depend on each other through a negation"},
		{"bad pointer", `a(X) :- attr(X, "drug", _).`, `attr: json pointer: "drug" must start with '/'`},
		{"undeclared pointer", `a(X) :- attr(X, "/dosage", _).`, "no Definition declares a field at /dosage (the types are health.blood-pressure, health.medication.dose-changed, health.medication.started"},
		{"pointer is not a string", `a(X) :- attr(X, 1, _).`, "attr: the pointer must be a string, got 1"},
		{"instant undefined", `a(X) :- current(X, _).`, "a reads instant/1, which is declared but has no rules or facts; define it in the query"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p, err := query.Parse(c.text)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			_, err = query.Compile(query.BaseSchema, defs, rules.Program, p)
			if !errors.Is(err, query.ErrInvalid) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Compile = %v, want ErrInvalid containing %q", err, c.want)
			}
		})
	}
}

// What is allowed: recursion through positive atoms, negation of a lower stratum, a
// pointer any one type declares, a declared relation the query defines, and a query
// that never reaches an undefined declaration.
func TestCompile_Accepts(t *testing.T) {
	t.Parallel()
	defs := registry(t)
	rules, err := query.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	cases := []struct{ name, text string }{
		{"recursion", `r(X, Y) :- later(X, Y). r(X, Z) :- r(X, Y), later(Y, Z).`},
		{"negation of a lower stratum", `a(X) :- entry(X, _, _, _, _, _, _), not b(X). b(X) :- attr(X, "/drug", _).`},
		{"pointer declared by one type", `a(X) :- attr(X, "/systolic", _).`},
		{"instant defined by a fact", `a(X) :- current(X, _). instant("2026-01-01T00:00:00.000000000Z").`},
		{"instant defined by a rule", `a(X) :- current(X, _). instant(At) :- entry(_, _, At, _, _, _, _).`},
		{"instant not reached", `a(X) :- later(X, _).`},
		{"a bound variable pointer", `a(X, P) :- attr(X, P, _).`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p, err := query.Parse(c.text)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if _, err := query.Compile(query.BaseSchema, defs, rules.Program, p); err != nil {
				t.Errorf("Compile: %v", err)
			}
		})
	}
}

// The bundled rule set parses, declares its parameter, and is named, versioned and
// hashed over its text.
func TestBundled(t *testing.T) {
	t.Parallel()
	rs, err := query.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	if rs.Name != "anam" || rs.Version != 1 {
		t.Errorf("rule set is %s v%d, want anam v1", rs.Name, rs.Version)
	}
	if !strings.HasPrefix(rs.Hash, "sha256:") || len(rs.Hash) != len("sha256:")+64 {
		t.Errorf("hash = %q", rs.Hash)
	}
	if rs.Program.Declared["instant"] != 1 {
		t.Errorf("declared = %v, want instant/1", rs.Program.Declared)
	}
	heads := map[string]bool{}
	for _, r := range rs.Program.Rules {
		heads[r.Head.Name] = true
	}
	for _, want := range []string{"current", "displaced"} {
		if !heads[want] {
			t.Errorf("the bundled set defines no %s", want)
		}
	}
}
