package query_test

import (
	"context"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
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
