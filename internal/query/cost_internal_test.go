package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
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
		entry2("r1", "test.measure", day2("2026-01-01"), `{"at": "2026-01-01 06:30:00", "seq": 1, "count": 118, "source_time": {"value": "2026-01-01 06:30:00", "zone": "Europe/Berlin", "basis": "source"}}`),
		entry2("r2", "test.measure", day2("2026-01-02"), `{"at": "2026-01-02 06:30:00", "seq": 1, "count": 146, "source_time": {"value": "2026-01-02 06:30:00", "zone": "Europe/Berlin", "basis": "source"}}`),
		entry2("r3", "test.measure", day2("2026-01-03"), `{"at": "2026-01-03 06:30:00", "seq": 1, "count": 160, "source_time": {"value": "2026-01-03 06:30:00", "zone": "Europe/Berlin", "basis": "source"}}`),
	)
	defs := catalogtest.Engine.Definitions(t)
	text := `high(E) :- attr(E, "/count", S), S > 100.`
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
