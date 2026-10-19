package query_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
)

// The syntax is small and its errors name the line and what was expected.
func TestParse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, text string
		rules      int
		declared   map[string]int
		wantErr    string
	}{
		{"fact and rule", `a("x", 1). b(X) :- a(X, N), N > 0.`, 2, map[string]int{}, ""},
		{"comment and declaration", "% a comment\ninstant/1.\nc(X) :- instant(X).", 1, map[string]int{"instant": 1}, ""},
		{"negation and anonymous", `d(X) :- a(X, _), not b(X).`, 1, map[string]int{}, ""},
		{"every operator", `e(X) :- a(X, N), N = 1, N != 2, N < 3, N <= 3, N > 0, N >= 0.`, 1, map[string]int{}, ""},
		{"booleans and escapes", `f(X) :- a(X, true), a("say \"hi\"", false).`, 1, map[string]int{}, ""},
		{"negative and decimal numbers", `g(X) :- a(X, -1.5), a(X, 2e3).`, 1, map[string]int{}, ""},
		{"missing period", `a(X) :- b(X)`, 0, nil, `line 1: expected ".", got "end of program"`},
		{"unterminated string", `a("x).`, 0, nil, "unterminated string"},
		{"upper-case relation", `A(X) :- b(X).`, 0, nil, "line 1: expected a relation name"},
		{"zero arity declaration", `a/0.`, 0, nil, "arity must be a positive integer"},
		{"bad operator", `a(X) :- b(X), X ~ 1.`, 0, nil, `unexpected '~'`},
		{"term where atom expected", `a(X) :- 1.`, 0, nil, "expected a comparison operator"},
		{"empty atom", `a() :- b(X).`, 0, nil, "expected a variable or a constant"},
		{"second line", "a(X) :- b(X).\nc(X) :- d(X), .", 0, nil, "line 2:"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			p, err := query.Parse(c.text)
			if c.wantErr != "" {
				if !errors.Is(err, query.ErrSyntax) || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("Parse = %v, want ErrSyntax containing %q", err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if len(p.Rules) != c.rules {
				t.Errorf("%d rules, want %d", len(p.Rules), c.rules)
			}
			if len(p.Declared) != len(c.declared) {
				t.Errorf("declared %v, want %v", p.Declared, c.declared)
			}
			for name, n := range c.declared {
				if p.Declared[name] != n {
					t.Errorf("declared %s/%d, want /%d", name, p.Declared[name], n)
				}
			}
		})
	}
}

// Parsed terms keep what was written: a string's text, a number's value, a
// variable's name.
func TestParse_Terms(t *testing.T) {
	t.Parallel()
	p, err := query.Parse(`r(X, _, "s", 1.50, true) :- base(X).`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	args := p.Rules[0].Head.Args
	if args[0].Var != "X" || args[1].Var != "_" {
		t.Errorf("variables = %q %q, want X _", args[0].Var, args[1].Var)
	}
	if args[2].Const.Kind() != query.KindString || args[2].Const.Text() != "s" {
		t.Errorf("string = %v", args[2].Const)
	}
	if args[3].Const.Kind() != query.KindNumber || args[3].Const.Text() != "1.5" {
		t.Errorf("number = %v, want 1.5", args[3].Const.Text())
	}
	if args[4].Const.Kind() != query.KindBool || args[4].Const.Text() != "true" {
		t.Errorf("bool = %v", args[4].Const)
	}
	if p.Rules[0].Line != 1 || p.Rules[0].Body[0].Atom.Name != "base" {
		t.Errorf("rule = %+v", p.Rules[0])
	}
}
