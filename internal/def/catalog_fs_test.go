package def_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/mattjoyce/anam/internal/def"
)

// copyCatalogToDisk copies a catalog root to dir, adding a .DS_Store and a notes/
// directory. It returns the copy as an os.DirFS.
func copyCatalogToDisk(t *testing.T, src fs.FS, dir string) fs.FS {
	t.Helper()
	add := func(path string, data []byte) {
		dst := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, data, 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}
	if err := fs.WalkDir(src, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(src, path)
		if err != nil {
			return err
		}
		add(path, raw)
		return nil
	}); err != nil {
		t.Fatalf("walk catalog: %v", err)
	}
	// Add a .DS_Store file (non-JSON, should be ignored).
	add(".DS_Store", []byte("mock ds_store data"))
	// Add a notes/ directory with a JSON file in it (non-regular at root, and the
	// directory should be ignored).
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes", "extra.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write notes/extra.json: %v", err)
	}
	return os.DirFS(dir)
}

// TestFS_ChangedManifestMovesSetVersion verifies that changing a manifest's bytes
// changes the set version.
func TestFS_ChangedManifestMovesSetVersion(t *testing.T) {
	fixtureSV, err := def.SetVersionFS(fixtureDefs())
	if err != nil {
		t.Fatalf("SetVersionFS(fixture): %v", err)
	}

	dir := t.TempDir()
	_ = copyCatalogToDisk(t, fixtureDefs(), dir)

	// Read one manifest, change its bytes, write it back.
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("glob manifests: %v (%d)", err, len(names))
	}
	first := names[0]
	raw, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read %s: %v", first, err)
	}
	raw = append(raw, []byte(" ")...)
	if err := os.WriteFile(first, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", first, err)
	}

	changedSV, err := def.SetVersionFS(os.DirFS(dir))
	if err != nil {
		t.Fatalf("SetVersionFS(changed): %v", err)
	}
	if changedSV == fixtureSV {
		t.Errorf("SetVersionFS did not change after modifying a manifest: %s", changedSV)
	}
}

// TestFS_BadManifestNameFails verifies that a manifest whose file name does not
// match its declared type is refused.
func TestFS_BadManifestNameFails(t *testing.T) {
	dir := t.TempDir()
	_ = copyCatalogToDisk(t, fixtureDefs(), dir)

	// Rename one manifest to a mismatched name.
	names, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("glob manifests: %v (%d)", err, len(names))
	}
	first := names[0]
	raw, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read %s: %v", first, err)
	}
	oldName := filepath.Base(first)
	newName := "wrong-name.json"
	if err := os.WriteFile(filepath.Join(dir, newName), raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", newName, err)
	}
	if err := os.Remove(first); err != nil {
		t.Fatalf("remove %s: %v", oldName, err)
	}

	if _, err := def.BundledFS(os.DirFS(dir)); !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("BundledFS with mismatched name = %v, want ErrBadManifest", err)
	}
	if _, err := def.SetVersionFS(os.DirFS(dir)); err != nil {
		t.Fatalf("SetVersionFS should not fail on bad manifest name: %v", err)
	}
}

// TestFS_HistoryCanBeEmpty verifies that a catalog root with no history/ directory
// returns an empty history.
func TestFS_HistoryCanBeEmpty(t *testing.T) {
	// Build a minimal catalog with just one manifest, no history.
	fsys := fstest.MapFS{
		"test.fake.json": &fstest.MapFile{
			Data: []byte(`{"meta":{"facet_type":"test.fake","definition_version":1,"classification":"Normal","validation_mode":"strict","source_schema":"fake/reading","time_semantics":{"fact_time":{"pointer":"/at","layout":"2006-01-02","zone":"UTC"}},"identity_key":["/at"]},"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["at"],"properties":{"at":{"type":"string","minLength":1}},"additionalProperties":false}}`),
		},
	}

	history, err := def.HistoryFS(fsys)
	if err != nil {
		t.Fatalf("HistoryFS with no history/ dir: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("HistoryFS = %d entries, want 0", len(history))
	}

	manifests, err := def.ManifestsFS(fsys)
	if err != nil {
		t.Fatalf("ManifestsFS with no history: %v", err)
	}
	if len(manifests) != 1 {
		t.Errorf("ManifestsFS = %d entries, want 1", len(manifests))
	}
}

// TestFS_IgnoresNonJSONFiles verifies that .DS_Store and notes/ are ignored.
func TestFS_IgnoresNonJSONFiles(t *testing.T) {
	dir := t.TempDir()
	copy := copyCatalogToDisk(t, fixtureDefs(), dir)

	// BundledFS should work exactly the same, ignoring .DS_Store and notes/.
	reg, err := def.BundledFS(copy)
	if err != nil {
		t.Fatalf("BundledFS with extra files: %v", err)
	}
	fixtureReg, err := def.BundledFS(fixtureDefs())
	if err != nil {
		t.Fatalf("BundledFS(fixture): %v", err)
	}
	all := reg.All()
	fixtureAll := fixtureReg.All()
	if len(all) != len(fixtureAll) {
		t.Errorf("BundledFS has %d types (fixture %d), ignoring extra files", len(all), len(fixtureAll))
	}
	for i := range fixtureAll {
		if i >= len(all) {
			break
		}
		if all[i].Type != fixtureAll[i].Type || all[i].Version != fixtureAll[i].Version {
			t.Errorf("type mismatch at %d: %s v%d vs %s v%d", i, all[i].Type, all[i].Version, fixtureAll[i].Type, fixtureAll[i].Version)
		}
	}

	// Also verify ManifestsFS and SetVersionFS work.
	sv, err := def.SetVersionFS(copy)
	if err != nil {
		t.Fatalf("SetVersionFS with extra files: %v", err)
	}
	fixtureSV, err := def.SetVersionFS(fixtureDefs())
	if err != nil {
		t.Fatalf("SetVersionFS(fixture): %v", err)
	}
	if sv != fixtureSV {
		t.Errorf("SetVersionFS with extra files = %s, want %s", sv, fixtureSV)
	}
}
