package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 78: anam retract on an importer's source plans from its newest complete
// snapshot. A facet the run left out is proposed; one sent after it completed is not;
// with no complete snapshot there is nothing to plan from.
func TestRetract_PlansAnImportersSourceFromItsSnapshot(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	cfg := config.Config{
		LedgerPath: filepath.Join(dir, "ledger.db"), DBPath: filepath.Join(dir, "tracking.db"),
		PersonID:    "matt",
		BucketWidth: span.Default, Timeout: 5 * time.Second, Limits: config.DefaultLimits(),
	}
	l, err := ledger.Create(ctx, cfg.LedgerPath, cfg.PersonID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	g := gate.New(l, defs, gate.StaticScopes{"scale-import": {"health.weight"}}, clock.Real{}, cfg.PersonID, gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	admit := func(id string) string {
		t.Helper()
		env := facet.Envelope{Type: "health.weight", Source: "scale-import", Injector: "scale-import",
			Payload: []byte(fmt.Sprintf(`{"id": %q, "date": "2026-10-09 07:30", "kg": 80.5}`, id))}
		b, err := g.Resolve(env)
		if err != nil {
			t.Fatal(err)
		}
		res, _, err := g.Commit(ctx, env, b)
		if err != nil {
			t.Fatal(err)
		}
		return res.FacetID
	}
	a, b, c := admit("w-1"), admit("w-2"), admit("w-3")

	buckets, _ := cfg.Buckets()
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	if _, _, _, err := planFromSnapshot(ctx, cmd, cfg, "scale-import"); err == nil ||
		!strings.Contains(err.Error(), "no complete snapshot") {
		t.Fatalf("with no snapshot: %v", err)
	}

	w, _ := l.Head(ctx)
	if _, err := db.RecordSnapshotPart(ctx, ops.SnapshotPart{ID: "run", Source: "scale-import",
		Types: []string{"health.weight"}, Part: 1, Parts: 1, FacetIDs: []string{a, c}}, time.Now(), int64(w), ops.SnapshotRetention{Incomplete: config.DefaultSnapshotTTL, Complete: config.DefaultSnapshotKeep}); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()
	d := admit("w-4") // sent after the run completed, by an incremental run

	absent, total, held, err := planFromSnapshot(ctx, cmd, cfg, "scale-import")
	if err != nil {
		t.Fatalf("planFromSnapshot: %v", err)
	}
	if len(absent) != 1 || absent[0].ID != b || total != 4 || held != 2 {
		t.Fatalf("plan = %d absent (%v), %d live, %d held; want only %s of 4, 2 held", len(absent), absent, total, held, b)
	}
	if absent[0].ID == d {
		t.Error("a facet sent after the snapshot was proposed")
	}
	if !strings.Contains(out.String(), "planned from snapshot run") {
		t.Errorf("report does not name the snapshot: %q", out.String())
	}
}
