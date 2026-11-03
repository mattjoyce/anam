package query_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/query"
)

// The catalogue says what can be asked: the base relations and the parameter, the
// instant lexeme, every type's pointers at every held version, and the bundled rules
// with their docs, from the Definitions and the rules alone. The Definitions are the
// engine fixture's, whose test.measure is at v2 and holds v1 in its history.
func TestDescribe_FromTheBundledDefinitions(t *testing.T) {
	t.Parallel()
	defs := catalogtest.Engine.Definitions(t)
	held := heldFixture(t)
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
	var measure query.TypeAttributes
	for _, ta := range cat.Types {
		if ta.Type == "test.measure" {
			measure = ta
		}
	}
	if measure.Current != 2 || len(measure.Versions) != 2 {
		t.Fatalf("test.measure = current v%d, %d versions; want v2 with two versions", measure.Current, len(measure.Versions))
	}
	// v2 added source_time; v1, held only in the history, has its count and not that.
	for _, v := range measure.Versions {
		ptrs := strings.Join(v.Pointers, " ")
		if !strings.Contains(ptrs, "/count") {
			t.Errorf("v%d pointers lack the count: %v", v.Version, v.Pointers)
		}
		if got := strings.Contains(ptrs, "/source_time/value"); got != (v.Version == 2) {
			t.Errorf("v%d declares /source_time/value = %t: %v", v.Version, got, v.Pointers)
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
	held := heldFixture(t)
	cases := []struct {
		text string
		want string
	}{
		{`a(E) :- attr(E, "/source_time/value", _).`, "test.measure declares /source_time/value at"},
		{`a(N) :- N = count : { attr(E, "/source_time/value", _) }.`, "test.measure declares /source_time/value at"},
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
	// A reader sets this bound as max_read_bytes, and the message says so.
	if !strings.Contains(err.Error(), "(max_read_bytes)") || strings.Contains(err.Error(), "Options") {
		t.Errorf("message = %q, want the bound named max_read_bytes", err)
	}
}

// heldFixture is the engine fixture's manifests and their history as a Held.
func heldFixture(t *testing.T) query.Held {
	t.Helper()
	defs, _ := fixture.Engine()
	manifests, err := def.ManifestsFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	return query.HeldOf(manifests)
}

// A historical version is described from its schema alone, nested pointers included,
// even when the body is no longer a contract Parse accepts. The engine fixture's history
// is flat, so this held set swaps test.measure's v1 for a body with a nested number
// and a validation mode no current contract may declare; the fixture's files and V are
// untouched.
func TestDescribe_AHistoricalVersionIsReadFromItsSchema(t *testing.T) {
	t.Parallel()
	const v1 = `{
  "meta": {
    "facet_type": "test.measure", "definition_version": 1, "classification": "Normal",
    "validation_mode": "lenient", "source_schema": "fixture/test.measure",
    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02 15:04:05", "zone": "Europe/Berlin"}},
    "identity_key": ["/at", "/seq"]
  },
  "schema": {"type": "object", "required": ["at", "seq", "reading"],
    "properties": {"at": {"type": "string"}, "seq": {"type": "number"},
      "reading": {"type": "object", "properties": {"value": {"type": "number"}, "unit": {"type": "string"}}}}}
}`
	if _, err := def.Parse([]byte(v1)); err == nil {
		t.Fatal("the historical body parses as a contract; the case needs one that is schema only")
	}
	held := heldFixture(t)
	held["test.measure"] = map[int][]byte{1: []byte(v1), 2: held["test.measure"][2]}
	cat, err := query.Describe(catalogtest.Engine.Definitions(t), held)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	var got []string
	for _, ta := range cat.Types {
		if ta.Type != "test.measure" {
			continue
		}
		for _, v := range ta.Versions {
			if v.Version == 1 {
				got = v.Pointers
			}
		}
	}
	if want := "/at /reading /reading/unit /reading/value /seq"; strings.Join(got, " ") != want {
		t.Errorf("test.measure v1 pointers = %v, want %s", got, want)
	}
}
