package query_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/query"
)

// A small Definition set: the regimen family, a point type with a schema, and a
// verbatim type that declares no fields.
func medManifest(typ string, closes bool) string {
	return `{
  "meta": {
    "facet_type": "` + typ + `", "definition_version": 1, "classification": "Sensitive",
    "validation_mode": "strict", "source_schema": "records/` + typ + `",
    "time_semantics": {"fact_time": {"pointer": "/date", "layout": "2006-01-02", "zone": "UTC"}},
    "identity_key": ["/id"],
    "series": {"name": "med.regimen", "key": ["/drug", "/course"], "closes": ` + map[bool]string{true: "true", false: "false"}[closes] + `}
  },
  "schema": {"type": "object", "required": ["id", "date", "drug", "course"],
    "properties": {"id": {"type": "string", "minLength": 1}, "date": {"type": "string", "minLength": 1}, "drug": {"type": "string", "minLength": 1}, "course": {"type": "number"}, "dose": {"type": "string", "minLength": 1}},
    "additionalProperties": false}
}`
}

const bpManifest = `{
  "meta": {
    "facet_type": "health.blood-pressure", "definition_version": 1, "classification": "Normal",
    "validation_mode": "strict", "source_schema": "bp-csv/reading",
    "time_semantics": {"fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
    "identity_key": ["/measured_at"]
  },
  "schema": {"type": "object", "required": ["measured_at", "systolic"],
    "properties": {"measured_at": {"type": "string", "minLength": 1}, "systolic": {"type": "number"}, "diastolic": {"type": "number"},
      "tags": {"type": "array", "items": {"type": "string"}}, "cuff": {"type": "object", "properties": {"arm": {"type": "string"}}, "additionalProperties": false}},
    "additionalProperties": false}
}`

const weightManifest = `{
  "meta": {
    "facet_type": "health.weight", "definition_version": 1, "classification": "Normal",
    "validation_mode": "verbatim", "source_schema": "health-db/weight",
    "time_semantics": {"fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
    "identity_key": ["/measured_at"]
  }
}`

func registry(t *testing.T) *def.Registry {
	t.Helper()
	var defs []def.Definition
	for _, m := range []string{medManifest("med.started", false), medManifest("med.dose-changed", false), medManifest("med.stopped", true), bpManifest, weightManifest} {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatalf("def.Parse: %v", err)
		}
		defs = append(defs, d)
	}
	reg, err := def.NewRegistry(defs...)
	if err != nil {
		t.Fatalf("def.NewRegistry: %v", err)
	}
	return reg
}

// entry is a value entry with a payload, the way a served value holds one.
func entry(id, typ, factTime, series, payload string) fold.Entry {
	return fold.Entry{
		FacetID: id, Type: facet.Type(typ), FactTime: factTime, Series: series,
		Source: "self-log", SourceRef: facet.SourceRef("self-log", id),
		Status: facet.StatusAdmitted, Classification: facet.Classification("Normal"),
		ContentHash: "sha256:" + id, Payload: json.RawMessage(payload),
	}
}

func day(d string) string { return d + "T00:00:00.000000000Z" }

// value builds a value and the versions map a fold would have given it, every
// facet at Definition version 1.
func value(t *testing.T, entries ...fold.Entry) (fold.Value, map[string]int) {
	t.Helper()
	v, err := fold.NewValue(entries)
	if err != nil {
		t.Fatalf("NewValue: %v", err)
	}
	versions := map[string]int{}
	for _, e := range entries {
		versions[e.FacetID] = 1
	}
	return v, versions
}

// run compiles the bundled rules with text over v and returns the named relation.
func run(t *testing.T, v fold.Value, versions map[string]int, text, relation string) query.Answer {
	t.Helper()
	ans, err := tryRun(v, versions, registry(t), text, relation)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return ans
}

func tryRun(v fold.Value, versions map[string]int, defs *def.Registry, text, relation string) (query.Answer, error) {
	rules, err := query.Bundled()
	if err != nil {
		return query.Answer{}, err
	}
	prog, err := query.Parse(text)
	if err != nil {
		return query.Answer{}, err
	}
	base, err := query.NewBase(v, versions, defs)
	if err != nil {
		return query.Answer{}, err
	}
	plan, err := query.Compile(query.BaseSchema, defs, rules.Program, prog)
	if err != nil {
		return query.Answer{}, err
	}
	res, err := query.Evaluate(context.Background(), plan, base)
	if err != nil {
		return query.Answer{}, err
	}
	return res.Relation(relation)
}

// rows renders an answer as one string per tuple, for comparing against a table.
func rows(a query.Answer) []string {
	out := make([]string, 0, a.Len())
	for _, tup := range a.Tuples {
		parts := make([]string, len(tup))
		for i, c := range tup {
			parts[i] = c.Text()
		}
		out = append(out, strings.Join(parts, " "))
	}
	return out
}

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The evaluator reaches a fixpoint through recursion (semi-naive: a path found in
// one round is joined again in the next), stops, and removes duplicates.
func TestEvaluate_RecursionReachesTheFixpoint(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("a", "med.started", day("2026-01-01"), "", `{}`),
		entry("b", "med.started", day("2026-01-02"), "", `{}`),
		entry("c", "med.started", day("2026-01-03"), "", `{}`),
		entry("d", "med.started", day("2026-01-04"), "", `{}`),
	)
	ans := run(t, v, versions, `
		next(E, F) :- entry(E, _, T, _, _, _, _), entry(F, _, U, _, _, _, _), T < U, not between(E, F).
		between(E, F) :- entry(E, _, T, _, _, _, _), entry(F, _, U, _, _, _, _), entry(_, _, V, _, _, _, _), T < V, V < U.
		reaches(E, F) :- next(E, F).
		reaches(E, F) :- reaches(E, G), next(G, F).
	`, "reaches")
	want := []string{"a b", "a c", "a d", "b c", "b d", "c d"}
	if got := rows(ans); !same(got, want) {
		t.Errorf("reaches = %v, want %v", got, want)
	}
}

// Comparisons: numbers numerically, strings lexically, and a mixed pair is unequal
// and unordered. attr reads payload leaves, nested ones by their full pointer, and a
// null is no value.
func TestEvaluate_ComparisonsAndAttributes(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("r1", "health.blood-pressure", day("2026-01-01"), "", `{"measured_at": "x", "systolic": 118, "tags": ["am", "sitting"], "cuff": {"arm": "left"}}`),
		entry("r2", "health.blood-pressure", day("2026-01-02"), "", `{"measured_at": "x", "systolic": 146.0, "diastolic": null}`),
		entry("r3", "health.blood-pressure", day("2026-01-03"), "", `{"measured_at": "x", "systolic": 9}`),
	)
	cases := []struct {
		name, text, relation string
		want                 []string
	}{
		{"numeric order", `high(E) :- attr(E, "/systolic", S), S > 140.`, "high", []string{"r2"}},
		{"not lexical", `low(E) :- attr(E, "/systolic", S), S < 100.`, "low", []string{"r3"}},
		{"146.0 is 146", `exact(E) :- attr(E, "/systolic", 146).`, "exact", []string{"r2"}},
		{"nested pointer", `left(E) :- attr(E, "/cuff/arm", "left").`, "left", []string{"r1"}},
		{"array item", `tagged(E, X) :- attr(E, "/tags/1", X).`, "tagged", []string{"r1 sitting"}},
		{"null is absent", `dia(E) :- attr(E, "/diastolic", _).`, "dia", nil},
		{"string vs number", `odd(E) :- attr(E, "/systolic", S), S != "118".`, "odd", []string{"r1", "r2", "r3"}},
		{"unordered kinds", `never(E) :- attr(E, "/systolic", S), S < "z".`, "never", nil},
		{"version is a number", `v(E) :- entry(E, _, _, _, _, _, 1).`, "v", []string{"r1", "r2", "r3"}},
		{"facts", `kind("bp"). kind("sleep"). k(X) :- kind(X), X != "sleep".`, "k", []string{"bp"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if got := rows(run(t, v, versions, c.text, c.relation)); !same(got, c.want) {
				t.Errorf("%s = %v, want %v", c.relation, got, c.want)
			}
		})
	}
}

// An answer is a value: sorted, serialised once, hashed; and the same program over
// the same value hashes the same.
func TestAnswer_IsAValue(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("b", "med.started", day("2026-01-02"), "", `{}`),
		entry("a", "med.started", day("2026-01-01"), "", `{}`),
	)
	text := `ids(E, N) :- entry(E, _, _, _, _, _, N).`
	first, second := run(t, v, versions, text, "ids"), run(t, v, versions, text, "ids")
	if first.Hash() != second.Hash() {
		t.Error("two evaluations of one program over one value hash differently")
	}
	want := `{"relation":"ids","tuples":[["a",1],["b",1]]}`
	if string(first.Bytes()) != want {
		t.Errorf("answer bytes = %s, want %s", first.Bytes(), want)
	}
	if !strings.HasPrefix(first.Hash(), "sha256:") {
		t.Errorf("hash = %s, want sha256:…", first.Hash())
	}
	if _, err := tryRun(v, versions, registry(t), text, "nothing"); !errors.Is(err, query.ErrInvalid) {
		t.Errorf("an unnamed relation = %v, want ErrInvalid", err)
	}
}

// A cancelled context stops an evaluation rather than letting it run on.
func TestEvaluate_HonoursTheDeadline(t *testing.T) {
	t.Parallel()
	var entries []fold.Entry
	for i := 0; i < 200; i++ {
		entries = append(entries, entry(strings.Repeat("x", 3)+string(rune('0'+i%10))+string(rune('a'+i/10)), "med.started", day("2026-01-01"), "", `{}`))
	}
	v, versions := value(t, entries...)
	rules, _ := query.Bundled()
	prog, err := query.Parse(`pairs(E, F, G) :- entry(E, _, _, _, _, _, _), entry(F, _, _, _, _, _, _), entry(G, _, _, _, _, _, _).`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	defs := registry(t)
	base, err := query.NewBase(v, versions, defs)
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	plan, err := query.Compile(query.BaseSchema, defs, rules.Program, prog)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := query.Evaluate(ctx, plan, base); !errors.Is(err, context.Canceled) {
		t.Errorf("Evaluate on a cancelled context = %v, want context.Canceled", err)
	}
}
