package projection_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/projection"
)

// copyCatalogToDisk copies the catalog root at srcDir to t.TempDir(), adding a
// .DS_Store file and a notes/ directory. Returns the copy as an os.DirFS.
func copyCatalogToDisk(t *testing.T, srcDir string) fs.FS {
	t.Helper()
	dir := t.TempDir()

	// Walk the source directory.
	var walk func(parentSrc, parentDst string)
	walk = func(parentSrc, parentDst string) {
		entries, err := os.ReadDir(parentSrc)
		if err != nil {
			t.Fatalf("read %s: %v", parentSrc, err)
		}
		for _, e := range entries {
			srcPath := filepath.Join(parentSrc, e.Name())
			dstPath := filepath.Join(parentDst, e.Name())
			if e.IsDir() {
				if err := os.MkdirAll(dstPath, 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", dstPath, err)
				}
				walk(srcPath, dstPath)
				continue
			}
			raw, err := os.ReadFile(srcPath)
			if err != nil {
				t.Fatalf("read %s: %v", srcPath, err)
			}
			if err := os.WriteFile(dstPath, raw, 0o644); err != nil {
				t.Fatalf("write %s: %v", dstPath, err)
			}
		}
	}
	walk(srcDir, dir)

	// Add a .DS_Store file (non-JSON, should be ignored).
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("mock ds_store data"), 0o644); err != nil {
		t.Fatalf("write .DS_Store: %v", err)
	}
	// Add a notes/ directory with a JSON file (non-regular at root, ignored).
	notesDir := filepath.Join(dir, "notes")
	if err := os.MkdirAll(notesDir, 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(notesDir, "extra.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write notes/extra.json: %v", err)
	}

	return os.DirFS(dir)
}

// fixtureDir writes the projections root of fixture.Mattjoyce under t.TempDir() and
// returns its path, for the tests that edit a catalog on disk.
func fixtureDir(t *testing.T) string {
	t.Helper()
	_, projs := fixture.Mattjoyce()
	_, dir := fixture.Dirs(t, fstest.MapFS{}, projs)
	return dir
}

// TestBundledFS_MatchEmbedded proves a disk copy of fixture.Mattjoyce loads exactly
// as the catalog embedded in the binary does. It reads the embedded catalog on
// purpose, to tie the fixture to it, and goes when the embedded catalog does (card
// 255, R4b).
func TestBundledFS_MatchEmbedded(t *testing.T) {
	embeddedReg, err := projection.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}

	srcDir := fixtureDir(t)
	copy := copyCatalogToDisk(t, srcDir)

	reg, err := projection.BundledFS(copy)
	if err != nil {
		t.Fatalf("BundledFS: %v", err)
	}

	// Template names, versions and hashes must match.
	embeddedAll := embeddedReg.Held()
	all := reg.Held()
	if len(embeddedAll) != len(all) {
		t.Fatalf("Bundled has %d templates, copy has %d", len(embeddedAll), len(all))
	}
	for i := range embeddedAll {
		got, want := all[i], embeddedAll[i]
		if got.Name != want.Name || got.Version != want.Version || got.Hash != want.Hash {
			t.Errorf("template %d: embedded %s v%d %s, copy %s v%d %s",
				i, want.Name, want.Version, want.Hash, got.Name, got.Version, got.Hash)
		}
	}
}

// TestBundledFS_IgnoresNonJSONFiles verifies that .DS_Store and notes/ are ignored.
func TestBundledFS_IgnoresNonJSONFiles(t *testing.T) {
	srcDir := fixtureDir(t)
	copy := copyCatalogToDisk(t, srcDir)

	reg, err := projection.BundledFS(copy)
	if err != nil {
		t.Fatalf("BundledFS with extra files: %v", err)
	}
	fixtureAll := catalogtest.Projections(t).Held()
	all := reg.Held()
	if len(all) != len(fixtureAll) {
		t.Fatalf("BundledFS has %d templates (fixture %d), ignoring extra files", len(all), len(fixtureAll))
	}
	for i := range fixtureAll {
		if i >= len(all) {
			break
		}
		if all[i].Name != fixtureAll[i].Name || all[i].Version != fixtureAll[i].Version || all[i].Hash != fixtureAll[i].Hash {
			t.Errorf("template %d mismatch: %s v%d %s vs %s v%d %s",
				i, all[i].Name, all[i].Version, all[i].Hash, fixtureAll[i].Name, fixtureAll[i].Version, fixtureAll[i].Hash)
		}
	}
}

// TestBundledFS_HistoryCanBeEmpty verifies that a catalog root with no history/
// directory works.
func TestBundledFS_HistoryCanBeEmpty(t *testing.T) {
	minimal := []byte(`{"name":"test","version":1,"types":[],"statuses":[],"max_classification":"Normal","from":"2024-01-01T00:00:00.000000000Z","to":"2024-12-31T00:00:00.000000000Z"}`)
	fsys := fstest.MapFS{
		"test.json": &fstest.MapFile{Data: minimal},
	}

	reg, err := projection.BundledFS(fsys)
	if err != nil {
		t.Fatalf("BundledFS with no history: %v", err)
	}
	tmpl, err := reg.Lookup("test")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	if tmpl.Name != "test" || tmpl.Version != 1 {
		t.Errorf("template = %s v%d, want test v1", tmpl.Name, tmpl.Version)
	}
}

// TestBundledFS_BadTemplateNameFails verifies that a template whose file name does
// not match its declared name is refused.
func TestBundledFS_BadTemplateNameFails(t *testing.T) {
	srcDir := fixtureDir(t)
	dir := t.TempDir()

	// Copy the templates, but rename one to a mismatched name.
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("read %s: %v", srcDir, err)
	}
	var firstJSON string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			firstJSON = e.Name()
			break
		}
	}
	if firstJSON == "" {
		t.Fatal("no JSON files in the fixture catalog")
	}
	raw, err := os.ReadFile(filepath.Join(srcDir, firstJSON))
	if err != nil {
		t.Fatalf("read %s: %v", firstJSON, err)
	}
	// Copy all files except the first one.
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if e.Name() == firstJSON {
			// Write it with a wrong name.
			if err := os.WriteFile(filepath.Join(dir, "wrong-name.json"), raw, 0o644); err != nil {
				t.Fatalf("write wrong-name.json: %v", err)
			}
			continue
		}
		raw2, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), raw2, 0o644); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
	}

	_, err = projection.BundledFS(os.DirFS(dir))
	if !errors.Is(err, projection.ErrBadTemplate) {
		t.Errorf("BundledFS with mismatched name = %v, want ErrBadTemplate", err)
	}
}

// TestBundledFS_ChangedTemplateMovesHash verifies that changing a template's bytes
// changes its hash.
func TestBundledFS_ChangedTemplateMovesHash(t *testing.T) {
	srcDir := fixtureDir(t)

	// Get a simple reference hash for the unchanged set.
	originalReg, err := projection.BundledFS(os.DirFS(srcDir))
	if err != nil {
		t.Fatalf("BundledFS(original): %v", err)
	}

	// Copy the templates to a temp dir by directly copying files from srcDir.
	dir := t.TempDir()
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("read %s: %v", srcDir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			// Skip history for simplicity.
			continue
		}
		raw, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", e.Name(), err)
		}
	}

	// Read one template file, change the "version" field to a new value.
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("glob templates: %v (%d)", err, len(names))
	}
	first := names[0]
	raw, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read %s: %v", first, err)
	}
	// Replace the description field value so the template content changes.
	// (The version changes the template identity, so the manifest would be rejected.)
	modified := []byte(strings.Replace(string(raw), `"description": "`, `"description": "MODIFIED: `, 1))
	if string(modified) == string(raw) {
		t.Fatal("could not modify the template file")
	}
	if err := os.WriteFile(first, modified, 0o644); err != nil {
		t.Fatalf("write %s: %v", first, err)
	}

	changedReg, err := projection.BundledFS(os.DirFS(dir))
	if err != nil {
		t.Fatalf("BundledFS(changed): %v", err)
	}

	// Compare templates by name and version. The changed set has no history, so
	// compare only current (newest) templates.
	originalByKey := map[string]string{}
	for _, t := range originalReg.Held() {
		key := fmt.Sprintf("%s:%d", t.Name, t.Version)
		originalByKey[key] = t.Hash
	}
	anyChanged := false
	for _, tmpl := range changedReg.Held() {
		key := fmt.Sprintf("%s:%d", tmpl.Name, tmpl.Version)
		origHash, ok := originalByKey[key]
		if !ok {
			t.Errorf("template %s v%d is not in the original set", tmpl.Name, tmpl.Version)
			continue
		}
		if origHash != tmpl.Hash {
			anyChanged = true
		}
	}
	if !anyChanged {
		t.Error("no template hash changed after modifying a file")
	}
}
