package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

// A scan that cannot be interrupted still ends the run at its deadline.
func TestBoundedReturnsAtTheDeadline(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	began := time.Now()
	_, err := bounded(ctx, func() ([]facet.Envelope, error) { <-release; return nil, nil })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want the deadline", err)
	}
	if waited := time.Since(began); waited > time.Second {
		t.Fatalf("returned after %s", waited)
	}
}

func TestBoundedPassesThroughAScanThatFinishes(t *testing.T) {
	t.Parallel()
	want := errors.New("no such file")
	envs, err := bounded(context.Background(), func() ([]facet.Envelope, error) {
		return []facet.Envelope{{}}, want
	})
	if len(envs) != 1 || !errors.Is(err, want) {
		t.Fatalf("got %d envelopes, %v", len(envs), err)
	}
}
