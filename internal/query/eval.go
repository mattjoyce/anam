package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattjoyce/tarski"
)

// maxSteps bounds the work of one evaluation, and so an aggregate's memory, which
// max_read_bytes does not reach: an aggregate derives few bytes however much it reads.
// Not configurable: it is the engine's guard against a runaway program, not a size a
// reader chooses. Measured 2026-10-28 on a 14,014-facet ledger at about 175,000 steps
// a second, a scan of every attr costs 260,332 steps; the record holds about six times
// those facets, and 5,000,000 is three times that scan, about 30 s of work.
const maxSteps = 5_000_000

// Evaluate runs a plan over a base to its fixpoint, within maxBytes of derived text
// (zero for none) and maxSteps of work. A derivation over the bound is a *TooLarge,
// worded for the bound a reader sets, max_read_bytes.
func Evaluate(ctx context.Context, plan Plan, base Base, maxBytes int) (Result, error) {
	res, err := tarski.Evaluate(ctx, plan, base, tarski.Options{MaxBytes: maxBytes, MaxSteps: maxSteps})
	if err != nil {
		return Result{}, reword(err)
	}
	return res, nil
}

// reword turns an engine refusal into Anam's wording for the byte and step bounds.
func reword(err error) error {
	var tl *TooLarge
	if errors.As(err, &tl) && tl.Bound == tarski.BoundBytes {
		return readerBound{tl}
	}
	if errors.As(err, &tl) && tl.Bound == tarski.BoundSteps {
		return readerSteps{tl}
	}
	return err
}

// readerBound is the module's *TooLarge with Anam's wording: the bound is the
// daemon's max_read_bytes, not an option of the engine. errors.As and errors.Is
// still find the *TooLarge and ErrTooLarge beneath it.
type readerBound struct{ *TooLarge }

func (e readerBound) Error() string {
	return fmt.Sprintf("derivation too large: %d derived tuples encode to more than %d bytes (max_read_bytes), at %s",
		e.Tuples, e.MaxBytes, e.Relation)
}

func (e readerBound) Unwrap() error { return e.TooLarge }

// readerSteps is the module's *TooLarge with Anam's wording for a step refusal.
type readerSteps struct{ *TooLarge }

func (e readerSteps) Error() string {
	return fmt.Sprintf("derivation too large: %d steps, over the query's bound of %d steps, at %s",
		e.Steps, e.MaxSteps, e.Relation)
}

func (e readerSteps) Unwrap() error { return e.TooLarge }
