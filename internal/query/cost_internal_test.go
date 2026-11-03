package query

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/tarski"
)

func entry2(id, typ, factTime, payload string) fold.Entry {
	return fold.Entry{
		FacetID: id, Type: facet.Type(typ), FactTime: factTime, Series: "",
		Source: "self-log", SourceRef: facet.SourceRef("self-log", id),
		Status: facet.StatusAdmitted, Classification: facet.Classification("Normal"),
		ContentHash: "sha256:" + id, Payload: json.RawMessage(payload),
	}
}

func day2(d string) string { return d + "T00:00:00.000000000Z" }

func value2(t *testing.T, entries ...fold.Entry) (fold.Value, map[string]int) {
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

func TestCostBoundMatches(t *testing.T) {
	t.Parallel()
	v, versions := value2(t,
		entry2("r1", "test.nested", day2("2026-01-01"), `{"at": "2026-01-01 06:30:00", "reading": {"value": 118, "unit": "count"}}`),
		entry2("r2", "test.nested", day2("2026-01-02"), `{"at": "2026-01-02 06:30:00", "reading": {"value": 146, "unit": "count"}}`),
		entry2("r3", "test.nested", day2("2026-01-03"), `{"at": "2026-01-03 06:30:00", "reading": {"value": 160, "unit": "count"}}`),
	)
	defs := nestedDefinitions(t)
	// The value is a number one level down, as an Open mHealth body's is.
	text := `high(E) :- attr(E, "/reading/value", S), S > 100.`
	relation := "high"

	tuples, steps, _, err := Cost(context.Background(), v, versions, defs, text, relation)
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if tuples != 3 {
		t.Fatalf("tuples = %d, want the three readings over 100", tuples)
	}

	rules, err := Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	prep, err := prepare(rules, v, versions, defs, text, relation)
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := tarski.Evaluate(context.Background(), prep.plan, prep.base, tarski.Options{MaxSteps: steps}); err != nil {
		t.Fatalf("expected success at steps: %v", err)
	}
	_, err = tarski.Evaluate(context.Background(), prep.plan, prep.base, tarski.Options{MaxSteps: steps - 1})
	if err == nil {
		t.Fatalf("expected error")
	}
	var tl *tarski.TooLarge
	if !errors.As(err, &tl) {
		t.Fatalf("expected *tarski.TooLarge, got %T", err)
	}
	if tl.Bound != tarski.BoundSteps {
		t.Fatalf("bound = %s", tl.Bound)
	}
}

// nestedDefinitions is the engine fixture's definitions with test.nested beside them, a
// type whose number sits under an object, which no engine fixture type has.
func nestedDefinitions(t *testing.T) *def.Registry {
	t.Helper()
	nested, err := def.Parse([]byte(`{
  "meta": {
    "facet_type": "test.nested", "definition_version": 1, "classification": "Normal",
    "validation_mode": "strict", "source_schema": "test/nested",
    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02 15:04:05", "zone": "UTC"}},
    "identity_key": ["/at"]
  },
  "schema": {"type": "object", "required": ["at", "reading"],
    "properties": {"at": {"type": "string", "minLength": 1},
      "reading": {"type": "object", "required": ["value"], "properties": {"value": {"type": "number"}, "unit": {"type": "string"}}, "additionalProperties": false}},
    "additionalProperties": false}
}`))
	if err != nil {
		t.Fatal(err)
	}
	all := slices.Clone(catalogtest.Engine.Definitions(t).All())
	reg, err := def.NewRegistry(append(all, nested)...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}
