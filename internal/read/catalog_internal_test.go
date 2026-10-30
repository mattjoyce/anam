package read

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/query"
)

// changedCatalog is the embedded catalog loaded from a directory copy whose first
// current definition carries a trailing space, so its set version is not the
// binary's.
func changedCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
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
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type().IsRegular() && filepath.Ext(e.Name()) == ".json" {
			p := filepath.Join(dir, e.Name())
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, append(raw, ' '), 0o644); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	cat, err := catalog.Load(context.Background(), dir, "")
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return cat
}

// A receipt names the set version of the catalog it was served under, not the
// binary's; nil registries are the catalog's own.
func TestServeDepsNamesTheCatalogsSetVersion(t *testing.T) {
	cat := changedCatalog(t)
	embedded, err := def.SetVersion()
	if err != nil {
		t.Fatal(err)
	}
	if cat.SetVersion() == embedded {
		t.Fatal("fixture: the changed catalog has the embedded set version")
	}
	deps, err := ServeDeps(cat, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("ServeDeps: %v", err)
	}
	if deps.DefinitionSetVersion != cat.SetVersion() {
		t.Errorf("DefinitionSetVersion = %s, want the catalog's %s (embedded is %s)", deps.DefinitionSetVersion, cat.SetVersion(), embedded)
	}
	if deps.Definitions != cat.Definitions() || deps.Templates != cat.Projections() {
		t.Error("nil registries did not fall back to the catalog's")
	}
	if _, err := ServeDeps(nil, nil, nil, nil, nil, nil); !errors.Is(err, ErrNoCatalog) {
		t.Errorf("ServeDeps(nil catalog) = %v, want ErrNoCatalog", err)
	}
}

// A Source names its own registries or falls back to its catalog's; with neither it
// fails rather than reach for the binary's copy.
func TestSourceFallsBackToItsCatalog(t *testing.T) {
	cat := changedCatalog(t)
	src := Source{Catalog: cat}
	if got, err := src.definitions(); err != nil || got != cat.Definitions() {
		t.Errorf("definitions() = %p, %v; want the catalog's", got, err)
	}
	if got, err := src.projections(); err != nil || got != cat.Projections() {
		t.Errorf("projections() = %p, %v; want the catalog's", got, err)
	}
	held, err := src.held()
	if err != nil || !reflect.DeepEqual(held, query.HeldOf(cat.Manifests())) {
		t.Errorf("held() = %d types, %v; want the catalog's manifests", len(held), err)
	}

	var none Source
	if _, err := none.definitions(); !errors.Is(err, ErrNoCatalog) {
		t.Errorf("definitions() with no catalog = %v, want ErrNoCatalog", err)
	}
	if _, err := none.projections(); !errors.Is(err, ErrNoCatalog) {
		t.Errorf("projections() with no catalog = %v, want ErrNoCatalog", err)
	}
	if _, err := none.held(); !errors.Is(err, ErrNoCatalog) {
		t.Errorf("held() with no catalog = %v, want ErrNoCatalog", err)
	}
}
