package query_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/query"
)

// The catalogue says what can be asked: the base relations and the parameter, the
// instant lexeme, every type's pointers at every held version, and the bundled rules
// with their docs, from the Definitions and the rules alone.
func TestDescribe_FromTheBundledDefinitions(t *testing.T) {
	t.Parallel()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	held, err := query.HeldBundled()
	if err != nil {
		t.Fatal(err)
	}
	cat, err := query.Describe(defs, held)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	names := map[string]query.Relation{}
	for _, r := range cat.Relations {
		names[r.Name] = r
	}
	if names["entry"].Arity != 7 || names["instant"].Kind != "parameter" || names["attr"].Kind != "base" {
		t.Errorf("relations = %+v", cat.Relations)
	}
	if cat.Instant.Layout != "2006-01-02T15:04:05.000000000Z07:00" || !strings.HasSuffix(cat.Instant.Example, "Z") {
		t.Errorf("instant = %+v", cat.Instant)
	}
	var bp query.TypeAttributes
	for _, ta := range cat.Types {
		if ta.Type == "health.blood-pressure" {
			bp = ta
		}
	}
	if bp.Current != 7 || len(bp.Versions) != 7 {
		t.Fatalf("blood-pressure = current v%d, %d versions; want v7 with seven versions", bp.Current, len(bp.Versions))
	}
	for _, v := range bp.Versions {
		if v.Version == 5 && !strings.Contains(strings.Join(v.Pointers, " "), "/systolic_blood_pressure/value") {
			t.Errorf("v5 pointers lack the systolic value: %v", v.Pointers)
		}
	}
	rules := map[string]query.Relation{}
	for _, r := range cat.Rules.Relations {
		rules[r.Name] = r
	}
	if cur := rules["current"]; cur.Arity != 2 || cur.Args[0] != "E" || cur.Args[1] != "At" || !strings.Contains(cur.Doc, "fold.Snapshot") {
		t.Errorf("current = %+v", cur)
	}
	if cat.Rules.Name != "anam" || cat.Rules.Hash == "" || !strings.Contains(cat.Rules.Text, "instant/1.") {
		t.Errorf("rules = %s v%d %s", cat.Rules.Name, cat.Rules.Version, cat.Rules.Hash)
	}
	if !strings.Contains(cat.Syntax, ":-") {
		t.Error("the syntax paragraph is missing")
	}
	// The catalogue is wire-shaped: it round-trips through JSON.
	b, err := json.Marshal(cat)
	if err != nil {
		t.Fatal(err)
	}
	var back query.Catalogue
	if err := json.Unmarshal(b, &back); err != nil || len(back.Types) != len(cat.Types) {
		t.Errorf("catalogue does not round-trip: %v", err)
	}
}

// A pointer some versions of a type declare and others do not is warned about, by
// version; a pointer every version declares, or none, is not.
func TestPointerWarnings(t *testing.T) {
	t.Parallel()
	held, err := query.HeldBundled()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		text string
		want string
	}{
		{`a(E) :- attr(E, "/missed_medication", _).`, "health.blood-pressure declares /missed_medication at"},
		{`a(E) :- attr(E, "/nothing", _).`, ""},
		{`a(E, P) :- attr(E, P, _).`, ""},
	}
	for _, c := range cases {
		p, err := query.Parse(c.text)
		if err != nil {
			t.Fatal(err)
		}
		w, err := query.PointerWarnings([]query.Program{p}, held)
		if err != nil {
			t.Fatalf("PointerWarnings: %v", err)
		}
		joined := strings.Join(w, "\n")
		if c.want == "" && len(w) != 0 {
			t.Errorf("%s: warnings %v, want none", c.text, w)
		}
		if c.want != "" && !strings.Contains(joined, c.want) {
			t.Errorf("%s: warnings %v, want one containing %q", c.text, w, c.want)
		}
	}
}

// A constant in instant/1 must be the whole fact-time lexeme; a bare date is refused
// with the lexeme it should have been.
func TestCompile_InstantMustBeAFactTime(t *testing.T) {
	t.Parallel()
	defs := registry(t)
	rules, _ := query.Bundled()
	cases := []struct{ text, want string }{
		{`instant("2026-06-15"). a(E) :- current(E, _).`, "write the whole instant, as 2026-06-15T00:00:00.000000000Z"},
		{`instant("2026-06-15T00:00:00Z"). a(E) :- current(E, _).`, "is not a fact-time"},
		{`instant(1). a(E) :- current(E, _).`, "instant 1 is not a fact-time"},
		{`a(E) :- current(E, At), instant(At), At != "x".`, ""},
		{`a(E) :- entry(E, _, At, _, _, _, _), not instant("2026-06-15T00:00:00.000000000Z").`, ""},
	}
	for _, c := range cases {
		p, err := query.Parse(c.text)
		if err != nil {
			t.Fatal(err)
		}
		_, err = query.Compile(query.BaseSchema, defs, rules.Program, p)
		switch {
		case c.want == "" && err != nil && !errors.Is(err, query.ErrInvalid):
			t.Errorf("%s: %v", c.text, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s = %v, want %q", c.text, err, c.want)
		}
	}
}

// The derivation bound counts every derived tuple, so an intermediate relation that
// grows past it is refused before the answer is, naming the relation.
func TestEvaluate_BoundsTheDerivation(t *testing.T) {
	t.Parallel()
	v, versions, _, _ := regimenFixture(t)
	defs := registry(t)
	rules, _ := query.Bundled()
	prog, err := query.Parse(`instant(At) :- entry(_, _, At, _, _, _, _). small(E) :- current(E, At), At > "2026-12-31T00:00:00.000000000Z".`)
	if err != nil {
		t.Fatal(err)
	}
	base, err := query.NewBase(v, versions, defs, true)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := query.Compile(query.BaseSchema, defs, rules.Program, prog)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := query.Evaluate(t.Context(), plan, base, 0); err != nil {
		t.Fatalf("unbounded: %v", err)
	}
	_, err = query.Evaluate(t.Context(), plan, base, 200)
	var tl *query.TooLarge
	if !errors.As(err, &tl) || !errors.Is(err, query.ErrTooLarge) {
		t.Fatalf("bounded = %v, want *TooLarge", err)
	}
	if tl.MaxBytes != 200 || tl.Bytes <= 200 || tl.Relation == "" || tl.Relation == "small" {
		t.Errorf("refusal = %+v, want an intermediate relation over 200 bytes", tl)
	}
}

// A constant decodes from the JSON it encoded to.
func TestConst_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	in := `[["a",1,true,1.5,"2"]]`
	var tuples []query.Tuple
	if err := json.Unmarshal([]byte(in), &tuples); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(tuples)
	if err != nil || string(out) != in {
		t.Errorf("round trip = %s, %v; want %s", out, err, in)
	}
	kinds := []query.Kind{query.KindString, query.KindNumber, query.KindBool, query.KindNumber, query.KindString}
	for i, c := range tuples[0] {
		if c.Kind() != kinds[i] {
			t.Errorf("constant %d kind = %v, want %v", i, c.Kind(), kinds[i])
		}
	}
	var c query.Const
	if err := json.Unmarshal([]byte(`{"a":1}`), &c); !errors.Is(err, query.ErrSyntax) {
		t.Errorf("an object as a constant = %v, want ErrSyntax", err)
	}
}
