package query_test

import (
	"context"
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/tarski"
)

func TestCostMatchesRun(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("r1", "health.blood-pressure", day("2026-01-01"), "", `{"measured_at": "x", "systolic": 118}`),
		entry("r2", "health.blood-pressure", day("2026-01-02"), "", `{"measured_at": "x", "systolic": 146}`),
	)
	defs := registry(t)
	text := `high(E) :- attr(E, "/systolic", S), S > 140.`
	relation := "high"

	ans, _, err := query.Run(context.Background(), v, versions, defs, text, relation, 0)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	tuples, steps, bytes, err := query.Cost(context.Background(), v, versions, defs, text, relation)
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if tuples != len(ans.Tuples()) {
		t.Fatalf("tuples = %d, run = %d", tuples, len(ans.Tuples()))
	}
	if steps <= 0 || bytes <= 0 {
		t.Fatalf("steps=%d bytes=%d", steps, bytes)
	}
	tuples2, steps2, bytes2, err := query.Cost(context.Background(), v, versions, defs, text, relation)
	if err != nil {
		t.Fatalf("Cost2: %v", err)
	}
	if tuples2 != tuples || steps2 != steps || bytes2 != bytes {
		t.Fatalf("not stable: (%d,%d,%d) vs (%d,%d,%d)", tuples, steps, bytes, tuples2, steps2, bytes2)
	}
}

func TestCostBoundMatches(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("r1", "health.blood-pressure", day("2026-01-01"), "", `{"measured_at": "x", "systolic": 118}`),
		entry("r2", "health.blood-pressure", day("2026-01-02"), "", `{"measured_at": "x", "systolic": 146}`),
		entry("r3", "health.blood-pressure", day("2026-01-03"), "", `{"measured_at": "x", "systolic": 160}`),
	)
	defs := registry(t)
	text := `high(E) :- attr(E, "/systolic", S), S > 100.`
	relation := "high"

	tuples, steps, _, err := query.Cost(context.Background(), v, versions, defs, text, relation)
	if err != nil {
		t.Fatalf("Cost: %v", err)
	}
	if tuples == 0 {
		t.Fatalf("unexpected")
	}

	rules, err := query.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	prog, err := query.Parse(text)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	plan, err := query.Compile(query.BaseSchema, defs, rules.Program, prog)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	base, err := query.NewBase(v, versions, defs, query.ReadsAttr([]string{relation}, rules.Program, prog))
	if err != nil {
		t.Fatalf("NewBase: %v", err)
	}
	if _, err := tarski.Evaluate(context.Background(), plan, base, tarski.Options{MaxSteps: steps}); err != nil {
		t.Fatalf("expected success at steps: %v", err)
	}
	_, err = tarski.Evaluate(context.Background(), plan, base, tarski.Options{MaxSteps: steps - 1})
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
