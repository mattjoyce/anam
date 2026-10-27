package query

import (
	"errors"
	"testing"

	"github.com/mattjoyce/tarski"
)

func TestRewordStepWording(t *testing.T) {
	tl := &tarski.TooLarge{Bound: tarski.BoundSteps, Steps: 7, MaxSteps: 5, Relation: "r"}
	got := reword(tl)
	if got.Error() != "derivation too large: 7 steps, over the query's bound of 5 steps, at r" {
		t.Fatalf("got %q", got.Error())
	}
	if !errors.Is(got, tarski.ErrTooLarge) {
		t.Fatal("errors.Is(got, tarski.ErrTooLarge) = false")
	}
}
