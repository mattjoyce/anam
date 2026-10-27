package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/tarski"
)

func medManifest2(typ string, closes bool) string {
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

const bpManifest2 = `{
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

func registry2(t *testing.T) *def.Registry {
	t.Helper()
	var defs []def.Definition
	for _, m := range []string{medManifest2("health.medication.started", false), medManifest2("health.medication.dose-changed", false), medManifest2("health.medication.stopped", true), bpManifest2} {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatalf("def.Parse: %v", err)
		}
		defs = append(defs, d)
	}
	r, err := def.NewRegistry(defs...)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return r
}

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
		entry2("r1", "health.blood-pressure", day2("2026-01-01"), `{"measured_at": "x", "systolic": 118}`),
		entry2("r2", "health.blood-pressure", day2("2026-01-02"), `{"measured_at": "x", "systolic": 146}`),
		entry2("r3", "health.blood-pressure", day2("2026-01-03"), `{"measured_at": "x", "systolic": 160}`),
	)
	defs := registry2(t)
	text := `high(E) :- attr(E, "/systolic", S), S > 100.`
	relation := "high"

	tuples, steps, _, err := Cost(context.Background(), v, versions, defs, text, relation)
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if tuples == 0 {
		t.Fatalf("unexpected")
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
