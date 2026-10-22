package retract_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/retract"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// fixture is a real ledger and operational store under the test's own directory, and a
// gate over them. admit puts one health.weight in as source.
type fixture struct {
	l  *ledger.Ledger
	db *ops.DB
	g  *gate.Gate
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	l, err := ledger.Create(ctx, filepath.Join(dir, "ledger.db"), "matt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	buckets, err := config.Config{BucketWidth: span.Default}.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, filepath.Join(dir, "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	scopes := gate.StaticScopes{"workbench": {"health.weight"}, "scale": {"health.weight"}}
	g := gate.New(l, defs, scopes, clock.Real{}, "matt",
		gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	return fixture{l: l, db: db, g: g}
}

func (f fixture) admit(t *testing.T, source, id string) string {
	t.Helper()
	env := facet.Envelope{Type: "health.weight", Source: source, Injector: source,
		Payload: []byte(fmt.Sprintf(`{"id": %q, "date": "2026-10-09 07:30", "kg": 80.5}`, id))}
	b, err := f.g.Resolve(env)
	if err != nil {
		t.Fatal(err)
	}
	res, _, err := f.g.Commit(context.Background(), env, b)
	if err != nil {
		t.Fatal(err)
	}
	return res.FacetID
}

func TestPlanFacet(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newFixture(t)
	live := f.admit(t, "workbench", "w-1")
	dead := f.admit(t, "workbench", "w-2")
	if _, _, err := f.g.Retract(ctx, dead); err != nil {
		t.Fatal(err)
	}
	reconciled := f.admit(t, "scale", "w-3")
	w, _ := f.l.Head(ctx)
	if _, err := f.db.RecordSnapshotPart(ctx, ops.SnapshotPart{ID: "run", Source: "scale", Types: []string{"health.weight"},
		Part: 1, Parts: 1, FacetIDs: []string{reconciled}}, time.Now(), int64(w),
		ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name     string
		outright bool
		source   string
		id       string
		wantErr  error
	}{
		{"a live facet of a source with no snapshot", true, "workbench", live, nil},
		{"declared but no complete snapshot", false, "workbench", live, nil},
		{"another source's name", true, "scale", live, retract.ErrWrongSource},
		{"already tombstoned", true, "workbench", dead, retract.ErrNotLive},
		{"declared with a complete snapshot", false, "scale", reconciled, retract.ErrReconciled},
		{"a snapshot but no declaration", true, "scale", reconciled, nil},
		{"unknown facet", true, "workbench", "no-such-facet", ledger.ErrNotFound},
	}
	// The group returns once every parallel subtest has, so the head is read after them.
	t.Run("plans", func(t *testing.T) {
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				t.Parallel()
				plan, err := retract.PlanFacet(ctx, f.l, f.db, tt.outright, tt.source, tt.id)
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("err = %v, want %v", err, tt.wantErr)
				}
				if tt.wantErr == nil && (plan.Facet.ID != tt.id || plan.Facet.ArtifactVersion != 1 || plan.Status != facet.StatusAdmitted) {
					t.Errorf("plan = %+v", plan)
				}
			})
		}
	})
	if after, _ := f.l.Head(ctx); after != w {
		t.Errorf("planning moved W from %d to %d", w, after)
	}
}
