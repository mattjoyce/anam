package daemon

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 14, at the daemon (card 119): Open stores the bundled Definitions, and a later
// build whose keying differs from what the store last saw refuses to start, leaving
// the stored set as it was.
func TestOpenGuardsTheBundledDefinitions(t *testing.T) {
	t.Parallel()
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

// The guard persists the catalog it guarded, not the binary's own copy: a catalog read
// from a directory whose current manifest differs from the embedded one only in its
// bytes (a trailing space, so the keying is the same and the guard passes) is stored
// with those bytes.
func TestGuardPersistsTheCatalogItGuarded(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	root := def.EmbeddedFS()
	if err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, p), raw, 0o644)
	}); err != nil {
		t.Fatalf("copy the embedded definitions: %v", err)
	}
	const typ = "health.blood-pressure"
	path := filepath.Join(dir, typ+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changed := append(raw, ' ')
	if err := os.WriteFile(path, changed, 0o644); err != nil {
		t.Fatal(err)
	}
	cat, err := catalog.Load(ctx, dir, "")
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	current, ok := cat.Definitions().Lookup(typ)
	if !ok {
		t.Fatalf("the catalog has no %s", typ)
	}

	cfg := newConfig(t)
	buckets, _ := cfg.Buckets()
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if err := guardCatalog(ctx, db, cat); err != nil {
		t.Fatalf("guardCatalog: %v", err)
	}
	if err := persistCatalog(ctx, db, cat); err != nil {
		t.Fatalf("persistCatalog: %v", err)
	}
	rows, err := db.Definitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(cat.Manifests()) {
		t.Errorf("stored %d definitions, want the catalog's %d", len(rows), len(cat.Manifests()))
	}
	for _, r := range rows {
		if r.Type == typ && r.Version == current.Version {
			if r.Body != string(changed) {
				t.Errorf("stored %s v%d is not the guarded catalog's body (the embedded copy was stored instead)", typ, r.Version)
			}
			return
		}
	}
	t.Fatalf("no stored %s v%d", typ, current.Version)
}
