package read_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 100: a snapshot that left thousands of live facets uncarried would cost one
// query each under one request; the scope bounds how many Absent judges, and past the
// bound it says so rather than running them all.
func TestAbsent_StopsAtTheJudgeBound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	for i, ref := range []string{"a", "b", "c"} {
		f := absentFixture("scale", "health.weight", ref, time.Date(2026, 7, 1+i, 6, 0, 0, 0, time.UTC))
		ev := facet.AnamEvent{Kind: facet.EventAdmitted, FacetID: f.ID, AnamTime: time.Date(2026, 7, 10, 9, 0, 0, i, time.UTC),
			Seq: int64(i + 1), ArtifactVersion: 1}
		if err := l.Admit(ctx, ledger.Watermark(i), f, ev); err != nil {
			t.Fatalf("Admit %s: %v", ref, err)
		}
	}
	src := read.Source{Ledger: l}
	scope := read.SnapshotScope{Source: "scale", Types: []string{"health.weight"}, Carried: map[string]bool{}, StartedW: 10}

	absent, live, err := read.Absent(ctx, src, l, scope)
	if err != nil || len(absent) != 3 || live != 3 {
		t.Fatalf("unbounded: %d absent of %d live, %v", len(absent), live, err)
	}
	scope.MaxJudged = 2
	if _, _, err := read.Absent(ctx, src, l, scope); !errors.Is(err, read.ErrTooManyAbsent) {
		t.Errorf("bounded at 2 with 3 uncarried: %v, want ErrTooManyAbsent", err)
	}
	scope.MaxJudged = 3
	if absent, _, err := read.Absent(ctx, src, l, scope); err != nil || len(absent) != 3 {
		t.Errorf("bounded at 3 with 3 uncarried: %d, %v", len(absent), err)
	}
}

func absentFixture(source, ftype, ref string, at time.Time) facet.Facet {
	canonical, err := facet.Canonicalize([]byte(`{"kg": 80}`))
	if err != nil {
		panic(err)
	}
	sourceRef := facet.SourceRef(source, ref)
	return facet.Facet{
		ID: facet.NewID(sourceRef), Type: facet.Type(ftype), Confidence: 1,
		Classification: facet.ClassificationNormal, ArtifactVersion: 1, DefinitionVersion: 1,
		FactTime:   at,
		Binding:    facet.Binding{PersonID: "matt", Assurance: facet.AssuranceHigh},
		Provenance: facet.Provenance{Source: source, Injector: source, SourceRef: sourceRef},
		Descriptor: facet.Descriptor{
			SourceSchema: "test", SchemaVersion: 1, ContentHash: facet.ContentHash(canonical),
			PayloadKind: facet.PayloadVerbatim, MediaType: "application/json",
		},
		Payload: canonical,
	}
}
