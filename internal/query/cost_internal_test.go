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
		entry2("r1", "health.blood-pressure", day2("2026-01-01"), `{"date_excel": "2026-1-1 06:30:00", "reading": "1", "systolic_blood_pressure": {"value": 118, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 76, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-01-01T06:30:00+11:00"}, "source_time": {"value": "2026-1-1 06:30:00", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}}`),
		entry2("r2", "health.blood-pressure", day2("2026-01-02"), `{"date_excel": "2026-1-2 06:30:00", "reading": "1", "systolic_blood_pressure": {"value": 146, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 90, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-01-02T06:30:00+11:00"}, "source_time": {"value": "2026-1-2 06:30:00", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}}`),
		entry2("r3", "health.blood-pressure", day2("2026-01-03"), `{"date_excel": "2026-1-3 06:30:00", "reading": "1", "systolic_blood_pressure": {"value": 160, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 95, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-01-03T06:30:00+11:00"}, "source_time": {"value": "2026-1-3 06:30:00", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}}`),
	)
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	text := `high(E) :- attr(E, "/systolic_blood_pressure/value", S), S > 100.`
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
