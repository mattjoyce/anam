package ops_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// A hold is held by name, and the global hold holds every source but the one named.
func TestHold_IsHeldAndReleased(t *testing.T) {
	t.Parallel()
	d, _ := open(t)
	ctx := context.Background()

	if err := d.SetHold(ctx, "garmin", "2026-10-22T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	held, err := d.HasHold(ctx, "garmin")
	if err != nil || !held {
		t.Fatalf("HasHold(garmin) = %v, %v, want true", held, err)
	}
	held, err = d.HasHold(ctx, "bp-csv")
	if err != nil || held {
		t.Fatalf("HasHold(bp-csv) = %v, %v, want false", held, err)
	}

	// Holding a source twice keeps the hold it has: the moment is not a new fact.
	if err := d.SetHold(ctx, "garmin", "2026-10-23T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	holds, err := d.Holds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(holds) != 1 || holds[0].HeldAt != "2026-10-22T00:00:00.000000000Z" {
		t.Fatalf("Holds = %+v, want one hold at the first moment", holds)
	}

	// The global hold holds every source.
	if err := d.SetHold(ctx, ops.HoldAll, "2026-10-22T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	for _, src := range []string{"garmin", "bp-csv", "anything"} {
		held, err := d.HasHold(ctx, src)
		if err != nil || !held {
			t.Fatalf("HasHold(%s) under the global hold = %v, %v, want true", src, held, err)
		}
	}

	// Releasing one source leaves the global hold.
	if err := d.ReleaseHold(ctx, "garmin"); err != nil {
		t.Fatal(err)
	}
	held, err = d.HasHold(ctx, "garmin")
	if err != nil || !held {
		t.Fatalf("HasHold(garmin) after releasing it under the global hold = %v, %v, want true", held, err)
	}

	// Releasing the global hold releases every hold.
	if err := d.ReleaseHold(ctx, ops.HoldAll); err != nil {
		t.Fatal(err)
	}
	holds, err = d.Holds(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(holds) != 0 {
		t.Fatalf("Holds = %+v, want none", holds)
	}
	held, err = d.HasHold(ctx, "garmin")
	if err != nil || held {
		t.Fatalf("HasHold(garmin) after the global hold was released = %v, %v, want false", held, err)
	}
}

// A held row keeps the moment it was held: it is not rewritten.
func TestHold_IsNotRewritten(t *testing.T) {
	t.Parallel()
	d, _ := open(t)
	ctx := context.Background()
	if err := d.SetHold(ctx, "garmin", "2026-10-22T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL().Exec(`UPDATE admission_hold SET held_at = '2026-10-23T00:00:00.000000000Z'`); err == nil {
		t.Error("an update of a held row was accepted")
	}
}

// A hold is authored state, so a migration carries it into the new store: a source
// held before the move is still held after it.
func TestHold_IsCarriedIntoAnotherStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	from, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = from.Close() }()
	if err := from.SetHold(ctx, "garmin", "2026-10-22T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	got, err := from.ReadRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Holds) != 1 || got.Holds[0].Source != "garmin" {
		t.Fatalf("ReadRegistered holds = %+v, want garmin held", got.Holds)
	}

	to, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = to.Close() }()
	if err := to.CarryRegistered(ctx, got); err != nil {
		t.Fatal(err)
	}
	held, err := to.HasHold(ctx, "garmin")
	if err != nil || !held {
		t.Fatalf("HasHold(garmin) in the new store = %v, %v, want true", held, err)
	}
	again, err := to.ReadRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Holds) != 1 || again.Holds[0] != got.Holds[0] {
		t.Fatalf("the new store holds %+v, want the old store's hold", again.Holds)
	}
}
