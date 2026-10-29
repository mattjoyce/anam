package def_test

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/mattjoyce/anam/internal/def"
)

// walkEmbedded copies the embedded catalog to t.TempDir(), adding a .DS_Store and a
// notes/ directory. It returns the copy as an os.DirFS.
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
	// Walk the embedded manifest FS. The embed root is the manifests/ directory.
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
		t.Fatalf("walk embedded catalog: %v", err)
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

func TestBundledFS_MatchEmbedded(t *testing.T) {
	embeddedReg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	embeddedSV, err := def.SetVersion()
	if err != nil {
		t.Fatalf("SetVersion: %v", err)
	}
	embeddedManifests, err := def.Manifests()
	if err != nil {
		t.Fatalf("Manifests: %v", err)
	}

	dir := t.TempDir()
	// The embedded catalog under manifests/ — which is what embedded() returns.
	// Walk the manifestFS at "manifests".
	copy := copyCatalogToDisk(t, fsDir(t, "manifests"), dir)

	reg, err := def.BundledFS(copy)
	if err != nil {
		t.Fatalf("BundledFS: %v", err)
	}
	sv, err := def.SetVersionFS(copy)
	if err != nil {
		t.Fatalf("SetVersionFS: %v", err)
	}
	manifests, err := def.ManifestsFS(copy)
	if err != nil {
		t.Fatalf("ManifestsFS: %v", err)
	}

	// The registries must be equivalent (same types, versions, keying).
	embeddedAll := embeddedReg.All()
	all := reg.All()
	if len(embeddedAll) != len(all) {
		t.Fatalf("Bundled has %d types, copy has %d", len(embeddedAll), len(all))
	}
	for i := range embeddedAll {
		got, want := all[i], embeddedAll[i]
		if got.Type != want.Type || got.Version != want.Version {
			t.Errorf("type %d: embedded %s v%d, copy %s v%d", i, want.Type, want.Version, got.Type, got.Version)
		}
	}

	// SetVersionFS must match.
	if sv != embeddedSV {
		t.Errorf("SetVersionFS = %s, want %s", sv, embeddedSV)
	}

	// ManifestsFS must produce the same type/version/body.
	if len(manifests) != len(embeddedManifests) {
		t.Fatalf("ManifestsFS has %d entries, embedded has %d", len(manifests), len(embeddedManifests))
	}
	for i := range embeddedManifests {
		got, want := manifests[i], embeddedManifests[i]
		if got.Type != want.Type || got.Version != want.Version || string(got.Body) != string(want.Body) {
			t.Errorf("manifest %d: embedded %s v%d, copy %s v%d, body matches %v", i, want.Type, want.Version, got.Type, got.Version, string(got.Body) == string(want.Body))
		}
	}
}

// fsDir returns an fs.FS rooted at the given subdirectory within the embedded
// manifestFS, for use in copying to disk.
func fsDir(t *testing.T, dir string) fs.FS {
	t.Helper()
	// Use the package's own test approach: access the embedded FS through a
	// test-only sub. We can't reference manifestFS from def_test, so we use
	// os.DirFS on the live manifests/ directory which is what the embedded FS
	// contains.
	// The manifests are at internal/def/manifests/ in the repo. Find them.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Walk up to find the module root.
	root := wd
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			t.Fatal("cannot find go.mod")
		}
		root = parent
	}
	src := filepath.Join(root, "internal", "def", dir)
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("embedded source %s: %v", src, err)
	}
	return os.DirFS(src)
}

// TestFS_ChangedManifestMovesSetVersion verifies that changing a manifest's bytes
// changes the set version.
func TestFS_ChangedManifestMovesSetVersion(t *testing.T) {
	embeddedSV, err := def.SetVersion()
	if err != nil {
		t.Fatalf("SetVersion: %v", err)
	}

	dir := t.TempDir()
	_ = copyCatalogToDisk(t, fsDir(t, "manifests"), dir)

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
	if changedSV == embeddedSV {
		t.Errorf("SetVersionFS did not change after modifying a manifest: %s", changedSV)
	}
}

// TestFS_BadManifestNameFails verifies that a manifest whose file name does not
// match its declared type is refused.
func TestFS_BadManifestNameFails(t *testing.T) {
	dir := t.TempDir()
	_ = copyCatalogToDisk(t, fsDir(t, "manifests"), dir)

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
		"health.fake.json": &fstest.MapFile{
			Data: []byte(`{"meta":{"facet_type":"health.fake","definition_version":1,"classification":"Normal","validation_mode":"strict","source_schema":"fake/reading","time_semantics":{"fact_time":{"pointer":"/at","layout":"2006-01-02","zone":"UTC"}},"identity_key":["/at"]},"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["at"],"properties":{"at":{"type":"string","minLength":1}},"additionalProperties":false}}`),
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
	copy := copyCatalogToDisk(t, fsDir(t, "manifests"), dir)

	// BundledFS should work exactly the same, ignoring .DS_Store and notes/.
	reg, err := def.BundledFS(copy)
	if err != nil {
		t.Fatalf("BundledFS with extra files: %v", err)
	}
	embeddedReg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	all := reg.All()
	embeddedAll := embeddedReg.All()
	if len(all) != len(embeddedAll) {
		t.Errorf("BundledFS has %d types (embedded %d), ignoring extra files", len(all), len(embeddedAll))
	}
	for i := range embeddedAll {
		if i >= len(all) {
			break
		}
		if all[i].Type != embeddedAll[i].Type || all[i].Version != embeddedAll[i].Version {
			t.Errorf("type mismatch at %d: %s v%d vs %s v%d", i, all[i].Type, all[i].Version, embeddedAll[i].Type, embeddedAll[i].Version)
		}
	}

	// Also verify ManifestsFS and SetVersionFS work.
	sv, err := def.SetVersionFS(copy)
	if err != nil {
		t.Fatalf("SetVersionFS with extra files: %v", err)
	}
	embeddedSV, err := def.SetVersion()
	if err != nil {
		t.Fatalf("SetVersion: %v", err)
	}
	if sv != embeddedSV {
		t.Errorf("SetVersionFS with extra files = %s, want %s", sv, embeddedSV)
	}
}

// TestBundledFS_ManifestsHistoryMatch verifies that ManifestsFS and HistoryFS
// return the same results as their embedded counterparts.
func TestBundledFS_ManifestsHistoryMatch(t *testing.T) {
	dir := t.TempDir()
	_ = copyCatalogToDisk(t, fsDir(t, "manifests"), dir)

	// ManifestsFS vs Manifests.
	want, err := def.Manifests()
	if err != nil {
		t.Fatalf("Manifests: %v", err)
	}
	got, err := def.ManifestsFS(os.DirFS(dir))
	if err != nil {
		t.Fatalf("ManifestsFS: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("ManifestsFS has %d entries, Manifests has %d", len(got), len(want))
	}
	slices.SortFunc(got, func(a, b def.Stored) int {
		if a.Type != b.Type {
			if a.Type < b.Type {
				return -1
			}
			return 1
		}
		return a.Version - b.Version
	})
	slices.SortFunc(want, func(a, b def.Stored) int {
		if a.Type != b.Type {
			if a.Type < b.Type {
				return -1
			}
			return 1
		}
		return a.Version - b.Version
	})
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Version != want[i].Version || string(got[i].Body) != string(want[i].Body) {
			t.Errorf("entry %d: ManifestsFS %s v%d, Manifests %s v%d, bodies match %v",
				i, got[i].Type, got[i].Version, want[i].Type, want[i].Version, string(got[i].Body) == string(want[i].Body))
		}
	}

	// HistoryFS vs History.
	wantHistory, err := def.History()
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	gotHistory, err := def.HistoryFS(os.DirFS(dir))
	if err != nil {
		t.Fatalf("HistoryFS: %v", err)
	}
	if len(gotHistory) != len(wantHistory) {
		t.Fatalf("HistoryFS has %d entries, History has %d", len(gotHistory), len(wantHistory))
	}
}
