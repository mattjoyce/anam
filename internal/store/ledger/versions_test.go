package ledger_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Versions is Version for many facets at once, across more than one batch: every ref
// comes back, each exactly the artifact Version returns, and a ref that names nothing
// is ErrNotFound rather than a short answer.
func TestVersions_IsVersionInBulk(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()

	const n = 1203 // more than two batches
	refs := make([]ledger.VersionRef, 0, n)
	for i := 0; i < n; i++ {
		f := testFacet(t, fmt.Sprintf("bp-csv:2026-02-01T06:%02d:%02d", i/60%60, i%60)+fmt.Sprint(i),
			fmt.Sprintf(`{"systolic": %d}`, 100+i%50), 1)
		if err := l.Admit(ctx, ledger.Watermark(i), f, admittedEvent(f, int64(i)+1)); err != nil {
			t.Fatalf("Admit %d: %v", i, err)
		}
		refs = append(refs, ledger.VersionRef{FacetID: f.ID, Version: 1})
	}

	got, err := l.Versions(ctx, refs)
	if err != nil {
		t.Fatalf("Versions: %v", err)
	}
	if len(got) != n {
		t.Fatalf("Versions returned %d of %d", len(got), n)
	}
	for _, f := range got[:5] {
		one, err := l.Version(ctx, f.ID, 1)
		if err != nil {
			t.Fatal(err)
		}
		if string(one.Payload.Bytes()) != string(f.Payload.Bytes()) || !one.FactTime.Equal(f.FactTime) {
			t.Errorf("Versions and Version disagree on %s", f.ID)
		}
	}

	bad := append(refs[:3:3], ledger.VersionRef{FacetID: refs[0].FacetID, Version: 2})
	if _, err := l.Versions(ctx, bad); !errors.Is(err, ledger.ErrNotFound) {
		t.Errorf("a ref to an absent version: %v, want ErrNotFound", err)
	}
	if got, err := l.Versions(ctx, nil); err != nil || len(got) != 0 {
		t.Errorf("no refs: %d, %v", len(got), err)
	}
}
