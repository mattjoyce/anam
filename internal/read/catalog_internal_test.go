package read

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/query"
)

// changedCatalog is the engine fixture loaded from a directory copy whose first
// current definition carries a trailing space, so its set version is not the
// fixture's. It returns the catalog and the unchanged fixture's set version. The
// fixture carries history (test.measure v1 among it), so held() is checked to keep
// earlier versions too.
func changedCatalog(t *testing.T) (*catalog.Catalog, string) {
	t.Helper()
	defs, projs := fixture.Engine()
	unchanged, err := def.SetVersionFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	defsDir, projsDir := fixture.Dirs(t, defs, projs)
	entries, err := os.ReadDir(defsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Type().IsRegular() && filepath.Ext(e.Name()) == ".json" {
			p := filepath.Join(defsDir, e.Name())
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, append(raw, ' '), 0o600); err != nil {
				t.Fatal(err)
			}
			break
		}
	}
	cat, err := catalog.Load(context.Background(), defsDir, projsDir)
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	return cat, unchanged
}

// A receipt names the set version of the catalog it was served under, not that of
// another catalog; nil registries are the catalog's own.
func TestServeDepsNamesTheCatalogsSetVersion(t *testing.T) {
	cat, unchanged := changedCatalog(t)
	if cat.SetVersion() == unchanged {
		t.Fatal("fixture: the changed catalog has the unchanged set version")
	}
	deps, err := ServeDeps(cat, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("ServeDeps: %v", err)
	}
	if deps.DefinitionSetVersion != cat.SetVersion() {
		t.Errorf("DefinitionSetVersion = %s, want the catalog's %s (unchanged is %s)", deps.DefinitionSetVersion, cat.SetVersion(), unchanged)
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
	cat, _ := changedCatalog(t)
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
	if _, ok := held["test.measure"][1]; !ok {
		t.Errorf("held() lacks test.measure v1, which the catalog holds only in its history")
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
