package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// copyCatalogRoot writes the files of an embedded catalog root into a new directory,
// history included, the way an operator would provision one.
func copyCatalogRoot(t *testing.T, root fs.FS) string {
	t.Helper()
	dir := t.TempDir()
	err := fs.WalkDir(root, ".", func(p string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return os.MkdirAll(filepath.Join(dir, p), 0o750)
		}
		b, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, p), b, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func openStore(t *testing.T, ctx context.Context, cfg config.Config) *ops.DB {
	t.Helper()
	buckets, err := cfg.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// A template whose file has left the projections directory is still held after the
// next start, so a receipt that names it still verifies; the store holds every
// version the catalog carried.
func TestOpenRetainsAProjectionAfterItsFileLeaves(t *testing.T) {
	ctx := context.Background()
	cfg := newConfig(t)
	cfg.ProjectionsDir = copyCatalogRoot(t, projection.EmbeddedFS())

	d, err := Open(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	held := d.projections.current.Load().Held()
	if len(held) == 0 {
		t.Fatal("the catalog holds no projections")
	}
	gone := held[0]
	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}

	db := openStore(t, ctx, cfg)
	rows, err := db.ProvisionedProjections(ctx)
	if err != nil || len(rows) != len(d.catalog.ProjectionManifests()) {
		t.Fatalf("stored %d provisioned projections, %v; want the catalog's %d", len(rows), err, len(d.catalog.ProjectionManifests()))
	}

	entries, err := os.ReadDir(cfg.ProjectionsDir)
	if err != nil {
		t.Fatal(err)
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		p := filepath.Join(cfg.ProjectionsDir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if tpl, err := projection.Parse(b); err == nil && tpl.Name == gone.Name {
			if err := os.Remove(p); err != nil {
				t.Fatal(err)
			}
			removed++
		}
	}
	if removed == 0 {
		t.Fatalf("no file for %s in the directory", gone.Name)
	}
	// History beside a template that is gone would not load, and the retained rows
	// are the point: the earlier versions leave with the file.
	if err := os.RemoveAll(filepath.Join(cfg.ProjectionsDir, "history")); err != nil {
		t.Fatal(err)
	}

	d2, err := Open(ctx, cfg, Options{})
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer func() { _ = d2.closeStores() }()
	if _, err := d2.catalog.Projections().Lookup(gone.Name); err == nil {
		t.Fatalf("the catalog still carries %s; the fixture removed nothing", gone.Name)
	}
	var found bool
	for _, v := range d2.projections.current.Load().Versions(gone.Name) {
		found = found || (v.Version == gone.Version && v.Hash == gone.Hash)
	}
	if !found {
		t.Errorf("%s v%d is not held after its file left the catalog", gone.Name, gone.Version)
	}
}

// A start refused by a check writes nothing of the catalog: the check runs before the
// store takes the catalog's Definitions and templates.
func TestOpenRefusedStartWritesNoCatalog(t *testing.T) {
	ctx := context.Background()
	cfg := newConfig(t)
	cfg.DefaultProjection = "no-such-projection"

	if _, err := Open(ctx, cfg, Options{}); err == nil {
		t.Fatal("Open with an unknown default_projection succeeded")
	}
	db := openStore(t, ctx, cfg)
	if rows, err := db.Definitions(ctx); err != nil || len(rows) != 0 {
		t.Errorf("a refused start stored %d definitions, %v", len(rows), err)
	}
	if rows, err := db.ProvisionedProjections(ctx); err != nil || len(rows) != 0 {
		t.Errorf("a refused start stored %d projections, %v", len(rows), err)
	}
}

// A registered Definition that contradicts the catalog's at the same type and version
// stops the start, and the catalog is not written.
func TestOpenRefusesARegisteredDefinitionTheCatalogContradicts(t *testing.T) {
	ctx := context.Background()
	cfg := newConfig(t)
	db := openStore(t, ctx, cfg)

	manifests, err := def.Manifests()
	if err != nil || len(manifests) == 0 {
		t.Fatalf("no embedded manifests: %v", err)
	}
	m := manifests[0]
	var body map[string]any
	if err := json.Unmarshal(m.Body, &body); err != nil {
		t.Fatal(err)
	}
	body["x_contradiction"] = true
	changed, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.RegisterDefinition(ctx, ops.DefinitionRow{Type: string(m.Type), Version: m.Version, Body: string(changed)}, "2026-10-30T00:00:00Z"); err != nil {
		t.Fatalf("RegisterDefinition: %v", err)
	}
	_ = db.Close()

	_, err = Open(ctx, cfg, Options{})
	if !errors.Is(err, def.ErrBadManifest) {
		t.Fatalf("Open = %v, want def.ErrBadManifest", err)
	}
	if !strings.Contains(err.Error(), string(m.Type)) {
		t.Errorf("error %q does not name %s", err, m.Type)
	}
	db2 := openStore(t, ctx, cfg)
	if rows, err := db2.Definitions(ctx); err != nil || len(rows) != 0 {
		t.Errorf("a refused start stored %d definitions, %v", len(rows), err)
	}
}

// One start event names where the catalog came from and its Definition set version.
func TestOpenLogsTheCatalogItAdmitted(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	cfg := newConfig(t)
	cfg.DefinitionsDir = copyCatalogRoot(t, def.EmbeddedFS())
	d, err := Open(context.Background(), cfg, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.closeStores() }()

	out := buf.String()
	for _, want := range []string{
		"catalog admitted",
		"definitions_source=" + cfg.DefinitionsDir,
		"projections_source=embedded",
		"definition_set_version=" + d.catalog.SetVersion(),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log %q lacks %q", out, want)
		}
	}
}

// A receipt minted on the built-in catalog verifies the same on a directory copied
// from it: same result, same notes, no "definition set has moved". Where the files came
// from never enters a content hash.
func TestReceiptVerifiesTheSameOnACopiedCatalog(t *testing.T) {
	cfg := readerConfig(t)

	d := open(t, cfg)
	srv := httptest.NewServer(d.Handler())
	var minted api.Published
	call(t, "GET", srv.URL+timelineBP, recorderToken, "", http.StatusOK, &minted)
	var embedded api.ReceiptVerified
	call(t, "GET", srv.URL+"/v1/receipts/"+minted.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &embedded)
	srv.Close()
	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}
	if !embedded.Result.OK() {
		t.Fatalf("the receipt does not verify on the built-in catalog: %s", embedded.Result.Summary())
	}

	cfg.DefinitionsDir = copyCatalogRoot(t, def.EmbeddedFS())
	cfg.ProjectionsDir = copyCatalogRoot(t, projection.EmbeddedFS())
	d2 := open(t, cfg)
	t.Cleanup(func() { _ = d2.closeStores() })
	srv2 := httptest.NewServer(d2.Handler())
	defer srv2.Close()
	var copied api.ReceiptVerified
	call(t, "GET", srv2.URL+"/v1/receipts/"+minted.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &copied)

	if copied.Result.OK() != embedded.Result.OK() || copied.Result.Summary() != embedded.Result.Summary() {
		t.Errorf("verification differs on a copied catalog:\n built-in: %s\n copied:   %s",
			embedded.Result.Summary(), copied.Result.Summary())
	}
	if !reflect.DeepEqual(copied.Result, embedded.Result) {
		t.Errorf("verification result differs on a copied catalog:\n built-in: %+v\n copied:   %+v", embedded.Result, copied.Result)
	}
}
