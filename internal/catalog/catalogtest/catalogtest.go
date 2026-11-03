// Package catalogtest builds the engine's catalog types over the fixture catalogs, for
// tests that need a registry, a catalog or a config naming one (card 255). Each
// fixture is a Fixture value, Engine or Personal, and each thing a Fixture builds is
// built once per process and shared: never modify a Definition, template or slice
// one returns.
//
// The package-level Definitions, Projections, Catalog and Config are Personal's,
// kept while tests move to Engine (R4b slices 4a to 4d), so no slice touches a file
// it is not converting. Slice 6 deletes them with Personal.
//
// Nothing outside a test imports it; see package fixture.
package catalogtest

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"sync"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/projection"
)

// Fixture builds registries, a catalog and configs over one fixture catalog.
type Fixture struct {
	roots       func() (definitions, projections fs.FS)
	definitions func() (*def.Registry, error)
	projections func() (*projection.Registry, error)
	loaded      func() (*catalog.Catalog, error)
}

// Engine is the fixture over fixture.Engine, the catalog the engine's own tests read.
var Engine = Over(fixture.Engine)

// Personal is the fixture over fixture.Mattjoyce, until R4b slice 6 deletes it.
var Personal = Over(fixture.Mattjoyce)

// Over is a Fixture over the catalog roots returns. Nothing is built until a test
// asks for it.
func Over(roots func() (definitions, projections fs.FS)) *Fixture {
	return &Fixture{
		roots: roots,
		definitions: sync.OnceValues(func() (*def.Registry, error) {
			defs, _ := roots()
			return def.BundledFS(defs)
		}),
		projections: sync.OnceValues(func() (*projection.Registry, error) {
			_, projs := roots()
			return projection.BundledFS(projs)
		}),
		loaded: sync.OnceValues(func() (*catalog.Catalog, error) { return load(roots) }),
	}
}

// load writes the catalog to a directory of its own, loads it the way the daemon
// loads a configured catalog, and removes the directory: Load reads every file into
// memory, so the catalog needs nothing on disk once it is built.
func load(roots func() (definitions, projections fs.FS)) (*catalog.Catalog, error) {
	dir, err := os.MkdirTemp("", "anam-catalogtest-")
	if err != nil {
		return nil, fmt.Errorf("catalogtest: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }() // a temp directory left behind is harmless
	defs, projs := roots()
	defsDir, projsDir, err := fixture.Write(dir, defs, projs)
	if err != nil {
		return nil, err
	}
	return catalog.Load(context.Background(), defsDir, projsDir)
}

// Definitions is the definition registry over the fixture's definitions.
func (f *Fixture) Definitions(t testing.TB) *def.Registry {
	t.Helper()
	r, err := f.definitions()
	if err != nil {
		t.Fatalf("catalogtest: definitions: %v", err)
	}
	return r
}

// Projections is the projection registry over the fixture's projections.
func (f *Fixture) Projections(t testing.TB) *projection.Registry {
	t.Helper()
	r, err := f.projections()
	if err != nil {
		t.Fatalf("catalogtest: projections: %v", err)
	}
	return r
}

// Catalog is the fixture loaded by catalog.Load from directories.
func (f *Fixture) Catalog(t testing.TB) *catalog.Catalog {
	t.Helper()
	c, err := f.loaded()
	if err != nil {
		t.Fatalf("catalogtest: catalog: %v", err)
	}
	return c
}

// Config points c at a copy of the fixture under the test's own directory, by
// setting DefinitionsDir and ProjectionsDir, as a person would in the config file.
func (f *Fixture) Config(t testing.TB, c *config.Config) {
	t.Helper()
	defs, projs := f.roots()
	c.DefinitionsDir, c.ProjectionsDir = fixture.Dirs(t, defs, projs)
}

// Definitions is Personal.Definitions.
func Definitions(t testing.TB) *def.Registry {
	t.Helper()
	return Personal.Definitions(t)
}

// Projections is Personal.Projections.
func Projections(t testing.TB) *projection.Registry {
	t.Helper()
	return Personal.Projections(t)
}

// Catalog is Personal.Catalog.
func Catalog(t testing.TB) *catalog.Catalog {
	t.Helper()
	return Personal.Catalog(t)
}

// Config is Personal.Config.
func Config(t testing.TB, c *config.Config) {
	t.Helper()
	Personal.Config(t, c)
}
