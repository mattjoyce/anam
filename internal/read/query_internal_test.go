package read

import (
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/query"
)

func TestAsReadErrorStepAndNumber(t *testing.T) {
	tlSteps := &query.TooLarge{Bound: query.BoundSteps, Steps: 7, MaxSteps: 5, Relation: "r", Tuples: 3}
	err := asReadError(tlSteps)
	var rl *TooLarge
	if !errors.As(err, &rl) {
		t.Fatalf("got %T, want *TooLarge", err)
	}
	if rl.Steps != 7 || rl.MaxSteps != 5 || rl.MaxBytes != 0 {
		t.Fatalf("Steps=%d MaxSteps=%d MaxBytes=%d", rl.Steps, rl.MaxSteps, rl.MaxBytes)
	}

	tlNum := &query.TooLarge{Bound: query.BoundNumber, Relation: "s"}
	err2 := asReadError(tlNum)
	if !errors.Is(err2, ErrQuery) {
		t.Fatalf("got %v, want bad-query error", err2)
	}
	var rl2 *TooLarge
	if errors.As(err2, &rl2) {
		t.Fatalf("BoundNumber refusal should not be a *TooLarge, got %+v", rl2)
	}
}
