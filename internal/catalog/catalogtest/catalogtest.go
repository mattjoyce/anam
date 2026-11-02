// Package catalogtest builds the engine's catalog types over fixture.Mattjoyce, for
// tests that need a registry, a catalog or a config naming one (card 255, R4a). Each
// is built once per process and shared: never modify a Definition, template or slice
// one returns.
//
// Nothing outside a test imports it; see package fixture.
package catalogtest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/projection"
)

var definitions = sync.OnceValues(func() (*def.Registry, error) {
	defs, _ := fixture.Mattjoyce()
	return def.BundledFS(defs)
})

var projections = sync.OnceValues(func() (*projection.Registry, error) {
	_, projs := fixture.Mattjoyce()
	return projection.BundledFS(projs)
})

// loaded is the fixture written to a directory of its own, loaded the way the daemon
// loads a configured catalog, and the directory removed: Load reads every file into
// memory, so the catalog needs nothing on disk once it is built.
var loaded = sync.OnceValues(func() (*catalog.Catalog, error) {
	dir, err := os.MkdirTemp("", "anam-catalogtest-")
	if err != nil {
		return nil, fmt.Errorf("catalogtest: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }() // a temp directory left behind is harmless
	defs, projs := fixture.Mattjoyce()
	defsDir, projsDir, err := fixture.Write(dir, defs, projs)
	if err != nil {
		return nil, err
	}
	return catalog.Load(context.Background(), defsDir, projsDir)
})

// Definitions is the definition registry over fixture.Mattjoyce.
func Definitions(t testing.TB) *def.Registry {
	t.Helper()
	r, err := definitions()
	if err != nil {
		t.Fatalf("catalogtest: definitions: %v", err)
	}
	return r
}

// Projections is the projection registry over fixture.Mattjoyce.
func Projections(t testing.TB) *projection.Registry {
	t.Helper()
	r, err := projections()
	if err != nil {
		t.Fatalf("catalogtest: projections: %v", err)
	}
	return r
}

// Catalog is fixture.Mattjoyce loaded by catalog.Load from directories.
func Catalog(t testing.TB) *catalog.Catalog {
	t.Helper()
	c, err := loaded()
	if err != nil {
		t.Fatalf("catalogtest: catalog: %v", err)
	}
	return c
}

// Config points c at a copy of fixture.Mattjoyce under the test's own directory, by
// setting DefinitionsDir and ProjectionsDir, as a person would in the config file.
func Config(t testing.TB, c *config.Config) {
	t.Helper()
	defs, projs := fixture.Mattjoyce()
	c.DefinitionsDir, c.ProjectionsDir = fixture.Dirs(t, defs, projs)
}
