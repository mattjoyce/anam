package daemon

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 14, at the daemon (card 119): Open stores the bundled Definitions, and a later
// build whose keying differs from what the store last saw refuses to start, leaving
// the stored set as it was.
func TestOpenGuardsTheBundledDefinitions(t *testing.T) {
	ctx := context.Background()
	cfg := newConfig(t)
	d, err := Open(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}

	buckets, _ := cfg.Buckets()
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	// The store holds the current body of every bundled type and the history the
	// binary carries (card 111).
	manifests, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Definitions(ctx)
	if err != nil || len(rows) != len(manifests) {
		t.Fatalf("stored %d definitions, %v; want %d", len(rows), err, len(manifests))
	}
	// Pretend the store last saw blood pressure keyed another way, at its newest
	// version: the guard compares the newest stored body with the current one.
	var bp ops.DefinitionRow
	for _, r := range rows {
		if r.Type == "health.blood-pressure" && r.Version > bp.Version {
			bp = r
		}
	}
	bp.Body = strings.Replace(bp.Body, `"identity_key": ["/date_excel", "/reading"]`, `"identity_key": ["/date_excel"]`, 1)
	if !strings.Contains(bp.Body, `"identity_key": ["/date_excel"]`) {
		t.Fatalf("fixture did not change the stored body")
	}
	if err := db.PutDefinitions(ctx, []ops.DefinitionRow{bp}); err != nil {
		t.Fatalf("PutDefinitions: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if d, err := Open(ctx, cfg, Options{}); !errors.Is(err, def.ErrKeyed) {
		if err == nil {
			_ = d.closeStores()
		}
		t.Fatalf("open against a differently keyed store = %v, want ErrKeyed", err)
	}
}
