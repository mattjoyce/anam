package catalog_test

// The tests in this file call catalog.Load with an empty directory, because the empty
// catalog is what they check (card 255, R4b): no directory selects no files, labelled
// "none". Each is listed in catalogtest's emptyOnPurpose, and passes every catalog it
// selects to a catalogtest Require helper, which TestTests_ReadNoEmbeddedCatalog checks.

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/def"
)

// emptyV is the definition set version of no files: SHA-256 over nothing.
const emptyV = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// TestLoadNoDirectoriesIsEmpty: with both directories empty Load returns a catalog
// with no definitions, projections or history, labelled none, with the empty V.
func TestLoadNoDirectoriesIsEmpty(t *testing.T) {
	t.Parallel()
	cat, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Load(none, none): %v", err)
	}
	catalogtest.RequireEmpty(t, cat)
	if cat.DefinitionsSource() != "none" || cat.ProjectionsSource() != "none" {
		t.Errorf("sources = %q, %q; want none, none", cat.DefinitionsSource(), cat.ProjectionsSource())
	}
	if cat.SetVersion() != emptyV {
		t.Errorf("SetVersion = %s, want %s", cat.SetVersion(), emptyV)
	}
	if n := len(cat.Definitions().All()); n != 0 {
		t.Errorf("Definitions().All() has %d entries, want none", n)
	}
	if n := len(cat.Projections().Held()); n != 0 {
		t.Errorf("Projections().Held() has %d entries, want none", n)
	}
	if n := len(cat.Manifests()) + len(cat.History()) + len(cat.ProjectionManifests()); n != 0 {
		t.Errorf("the empty catalog holds %d bodies, want none", n)
	}
	if got := cat.AbsentStructural(); len(got) != 2 || got[0] != "record.edge" || got[1] != "record.document" {
		t.Errorf("AbsentStructural = %v, want record.edge and record.document", got)
	}
}

// TestLoadOneDirectoryEmpty: each component is chosen on its own, so an empty one is
// the empty component beside a directory, never the binary's.
func TestLoadOneDirectoryEmpty(t *testing.T) {
	t.Parallel()
	minDefs, minProjs := fixture.Minimal()
	defs, projs := fixture.Dirs(t, minDefs, minProjs)
	full, err := catalog.Load(context.Background(), defs, projs)
	if err != nil {
		t.Fatal(err)
	}

	noProjs, err := catalog.Load(context.Background(), defs, "")
	if err != nil {
		t.Fatalf("Load(dir, none): %v", err)
	}
	catalogtest.RequireNoProjections(t, noProjs)
	if noProjs.DefinitionsSource() != defs || noProjs.ProjectionsSource() != "none" {
		t.Errorf("sources = %q, %q; want %s, none", noProjs.DefinitionsSource(), noProjs.ProjectionsSource(), defs)
	}
	if noProjs.SetVersion() != full.SetVersion() {
		t.Errorf("SetVersion = %s, want the directory's %s", noProjs.SetVersion(), full.SetVersion())
	}
	if n := len(noProjs.Projections().Held()); n != 0 {
		t.Errorf("no projections_dir holds %d projections, want none", n)
	}

	noDefs, err := catalog.Load(context.Background(), "", projs)
	if err != nil {
		t.Fatalf("Load(none, dir): %v", err)
	}
	catalogtest.RequireNoDefinitions(t, noDefs)
	if noDefs.DefinitionsSource() != "none" || noDefs.ProjectionsSource() != projs {
		t.Errorf("sources = %q, %q; want none, %s", noDefs.DefinitionsSource(), noDefs.ProjectionsSource(), projs)
	}
	if noDefs.SetVersion() != emptyV {
		t.Errorf("SetVersion = %s, want %s", noDefs.SetVersion(), emptyV)
	}
	if n := len(noDefs.Definitions().All()); n != 0 {
		t.Errorf("no definitions_dir holds %d definitions, want none", n)
	}
	if len(noDefs.Projections().Held()) != len(full.Projections().Held()) {
		t.Errorf("projections_dir holds %d projections beside no definitions, want %d",
			len(noDefs.Projections().Held()), len(full.Projections().Held()))
	}
}

// TestLoadEmptyIsTheSetVersionOfNoFiles: the empty V is SetVersionFS over an empty
// root, as an empty directory gives it.
func TestLoadEmptyIsTheSetVersionOfNoFiles(t *testing.T) {
	t.Parallel()
	v, err := def.SetVersionFS(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	if v != emptyV {
		t.Errorf("SetVersionFS(no files) = %s, want %s", v, emptyV)
	}
	dirCat, err := catalog.Load(context.Background(), t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if dirCat.SetVersion() != emptyV {
		t.Errorf("an empty directory's V = %s, want %s", dirCat.SetVersion(), emptyV)
	}
}

// TestLoadEmptyIsBuiltOnce: the empty catalog is shared, and a canceled context
// still refuses it, even once it is built.
func TestLoadEmptyIsBuiltOnce(t *testing.T) {
	t.Parallel()
	a, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	catalogtest.RequireEmpty(t, a)
	catalogtest.RequireEmpty(t, b)
	if a != b {
		t.Error("two loads of the empty catalog built two catalogs, want one shared")
	}
	for n := 1; n <= 2; n++ {
		cat, err := catalog.Load(&cancelAt{Context: context.Background(), n: n}, "", "")
		catalogtest.RequireRefused(t, cat, err)
		if cat != nil || !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, context.Canceled) {
			t.Errorf("cancel at check %d: cat %v, err %v, want nil and both sentinels", n, cat, err)
		}
	}
}
