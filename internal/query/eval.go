package query

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattjoyce/tarski"
)

// Evaluate runs a plan over a base to its fixpoint, within maxBytes of derived text
// (zero for none). A derivation over the bound is a *TooLarge, worded for the bound a
// reader sets, max_read_bytes.
func Evaluate(ctx context.Context, plan Plan, base Base, maxBytes int) (Result, error) {
	res, err := tarski.Evaluate(ctx, plan, base, tarski.Options{MaxBytes: maxBytes})
	if err != nil {
		var tl *TooLarge
		if errors.As(err, &tl) && tl.Bound == tarski.BoundBytes {
			return Result{}, readerBound{tl}
		}
		return Result{}, err
	}
	return res, nil
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
