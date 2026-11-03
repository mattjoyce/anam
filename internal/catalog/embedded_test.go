package catalog_test

// The tests in this file read the catalog embedded in the binary, by name, because
// the embedded path of Load is what they check: a disk copy of it loads the same
// catalog, and it is built once per process. Pointed at a fixture they would test the
// fixture and prove nothing about the binary. They are on card 255's allow-list and
// are rewritten when R4b empties the embedded catalog. Every other test in the package
// reads fixture.Mattjoyce.

import (
	"context"
	"errors"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/projection"
)

// ---------------------------------------------------------------------------
// TestLoadEmbeddedAndDiskCopy
// ---------------------------------------------------------------------------
func TestLoadEmbeddedAndDiskCopy(t *testing.T) {
	embedded, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Load(embedded, embedded): %v", err)
	}
	defDir, projDir := copyCatalog(t, def.EmbeddedFS(), projection.EmbeddedFS())

	mixed, err := catalog.Load(context.Background(), defDir, projDir)
	if err != nil {
		t.Fatalf("Load(disk+disk): %v", err)
	}

	// Compare definition registry contents.
	embeddedAll := embedded.Definitions().All()
	mixedAll := mixed.Definitions().All()
	if len(embeddedAll) != len(mixedAll) {
		t.Fatalf("embedded has %d definitions, disk copy has %d", len(embeddedAll), len(mixedAll))
	}
	for i := range embeddedAll {
		got, want := mixedAll[i], embeddedAll[i]
		if got.Type != want.Type || got.Version != want.Version {
			t.Errorf("definition %d: embedded %s v%d, copy %s v%d",
				i, want.Type, want.Version, got.Type, got.Version)
		}
	}

	// Compare projection registry contents.
	embeddedProj := embedded.Projections().Held()
	mixedProj := mixed.Projections().Held()
	if len(embeddedProj) != len(mixedProj) {
		t.Fatalf("embedded has %d projections, disk copy has %d", len(embeddedProj), len(mixedProj))
	}
	for i := range embeddedProj {
		got, want := mixedProj[i], embeddedProj[i]
		if got.Name != want.Name || got.Version != want.Version || got.Hash != want.Hash {
			t.Errorf("projection %d: embedded %s v%d %s, copy %s v%d %s",
				i, want.Name, want.Version, want.Hash, got.Name, got.Version, got.Hash)
		}
	}

	// Compare set versions.
	if embedded.SetVersion() != mixed.SetVersion() {
		t.Errorf("set version: embedded %s, disk copy %s", embedded.SetVersion(), mixed.SetVersion())
	}

	// Compare manifests (bodies included).
	embeddedMan := embedded.Manifests()
	mixedMan := mixed.Manifests()
	if len(embeddedMan) != len(mixedMan) {
		t.Fatalf("embedded has %d manifests, disk copy has %d", len(embeddedMan), len(mixedMan))
	}
	for i := range embeddedMan {
		got, want := mixedMan[i], embeddedMan[i]
		if got.Type != want.Type || got.Version != want.Version || string(got.Body) != string(want.Body) {
			t.Errorf("manifest %d: embedded %s v%d, copy %s v%d, body matches %v",
				i, want.Type, want.Version, got.Type, got.Version, string(got.Body) == string(want.Body))
		}
	}

	// Compare history.
	embeddedHist := embedded.History()
	mixedHist := mixed.History()
	if len(embeddedHist) != len(mixedHist) {
		t.Fatalf("embedded has %d history entries, disk copy has %d", len(embeddedHist), len(mixedHist))
	}
	for i := range embeddedHist {
		got, want := mixedHist[i], embeddedHist[i]
		if got.Type != want.Type || got.Version != want.Version || string(got.Body) != string(want.Body) {
			t.Errorf("history %d: embedded %s v%d, copy %s v%d",
				i, want.Type, want.Version, got.Type, got.Version)
		}
	}

	// Compare projection bodies, current then history.
	embeddedPM := embedded.ProjectionManifests()
	mixedPM := mixed.ProjectionManifests()
	if len(embeddedPM) != len(mixedPM) {
		t.Fatalf("embedded has %d projection bodies, disk copy has %d", len(embeddedPM), len(mixedPM))
	}
	for i := range embeddedPM {
		got, want := mixedPM[i], embeddedPM[i]
		if got.Name != want.Name || got.Version != want.Version || string(got.Body) != string(want.Body) {
			t.Errorf("projection body %d: embedded %s v%d, copy %s v%d", i, want.Name, want.Version, got.Name, got.Version)
		}
	}

	// Source labels: embedded vs disk.
	if embedded.DefinitionsSource() != "embedded" || embedded.ProjectionsSource() != "embedded" {
		t.Errorf("embedded sources: def=%q proj=%q", embedded.DefinitionsSource(), embedded.ProjectionsSource())
	}
	if mixed.DefinitionsSource() != defDir || mixed.ProjectionsSource() != projDir {
		t.Errorf("disk sources: def=%q (want %q) proj=%q (want %q)",
			mixed.DefinitionsSource(), defDir, mixed.ProjectionsSource(), projDir)
	}

	// Mixed: embedded defs, disk projs.
	mixed2, err := catalog.Load(context.Background(), "", projDir)
	if err != nil {
		t.Fatalf("Load(embedded, disk): %v", err)
	}
	if mixed2.DefinitionsSource() != "embedded" || mixed2.ProjectionsSource() != projDir {
		t.Errorf("mixed sources: def=%q proj=%q", mixed2.DefinitionsSource(), mixed2.ProjectionsSource())
	}
	if mixed2.SetVersion() != embedded.SetVersion() {
		t.Errorf("mixed set version %s != embedded %s", mixed2.SetVersion(), embedded.SetVersion())
	}

	// Mixed: disk defs, embedded projs.
	mixed3, err := catalog.Load(context.Background(), defDir, "")
	if err != nil {
		t.Fatalf("Load(disk, embedded): %v", err)
	}
	if mixed3.DefinitionsSource() != defDir || mixed3.ProjectionsSource() != "embedded" {
		t.Errorf("mixed sources: def=%q proj=%q", mixed3.DefinitionsSource(), mixed3.ProjectionsSource())
	}
	if mixed3.SetVersion() != embedded.SetVersion() {
		t.Errorf("mixed set version %s != embedded %s", mixed3.SetVersion(), embedded.SetVersion())
	}
}

// TestLoadEmbeddedIsBuiltOnce: the embedded catalog is shared, and a canceled
// context still refuses it, even once it is built.
func TestLoadEmbeddedIsBuiltOnce(t *testing.T) {
	a, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("two loads of the embedded catalog built two catalogs, want one shared")
	}
	for n := 1; n <= 2; n++ {
		cat, err := catalog.Load(&cancelAt{Context: context.Background(), n: n}, "", "")
		if cat != nil || !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, context.Canceled) {
			t.Errorf("cancel at check %d: cat %v, err %v, want nil and both sentinels", n, cat, err)
		}
	}
}
