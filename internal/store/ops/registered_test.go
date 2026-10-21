package ops_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 200: a migration reads the registered rows of one store in a transaction and
// writes them into another, moment of registration included, all or none.
func TestRegistered_ReadFromOneStoreIsWrittenToAnother(t *testing.T) {
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
	for v := 1; v <= 2; v++ {
		if err := from.RegisterDefinition(ctx, ops.DefinitionRow{Type: "test.reading", Version: v, Body: `{"v":` + string(rune('0'+v)) + `}`}, "2026-10-2"+string(rune('0'+v))+"T00:00:00.000000000Z"); err != nil {
			t.Fatal(err)
		}
	}
	if err := from.RegisterProjection(ctx, ops.ProjectionRow{Name: "heart", Version: 1, Body: `{"name":"heart"}`}, "2026-10-23T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if err := from.PutDefinitions(ctx, []ops.DefinitionRow{{Type: "bundled.type", Version: 1, Body: `{}`}}); err != nil {
		t.Fatal(err)
	}

	got, err := from.ReadRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Definitions) != 2 || len(got.Projections) != 1 || len(got.Provisioned) != 1 || got.Definitions[1].RegisteredAt != "2026-10-22T00:00:00.000000000Z" {
		t.Fatalf("ReadRegistered = %+v", got)
	}

	to, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = to.Close() }()
	if err := to.CarryRegistered(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, err := to.ReadRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Definitions) != 2 || again.Definitions[0] != got.Definitions[0] || again.Projections[0] != got.Projections[0] {
		t.Errorf("the new store holds %+v, want the old store's rows", again)
	}
	if len(again.Provisioned) != 0 {
		t.Errorf("the provisioned Definitions were written: %+v; the daemon provisions its own", again.Provisioned)
	}

	// Writing the rows again fails whole: the tables are immutable and the call is
	// one transaction, so a failed carry leaves nothing half-written.
	extra := got
	extra.Definitions = append([]ops.RegisteredDefinition{{DefinitionRow: ops.DefinitionRow{Type: "test.other", Version: 1, Body: `{}`}, RegisteredAt: "x"}}, got.Definitions...)
	if err := to.CarryRegistered(ctx, extra); err == nil {
		t.Fatal("a carry over rows already there succeeded")
	}
	after, err := to.ReadRegistered(ctx)
	if err != nil || len(after.Definitions) != 2 {
		t.Errorf("after the failed carry the store holds %d Definitions (%v), want the 2 it had", len(after.Definitions), err)
	}
}
