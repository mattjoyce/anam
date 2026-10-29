package projection

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// update rewrites the golden file instead of comparing, for a deliberate catalog
// change.
var update = flag.Bool("update", false, "rewrite testdata/baseline.golden instead of comparing")

// baselineLines hashes every file in a catalog root (fsys at "." and "history/") and
// returns the manifest lines, then appends the template lines from BundledFS(fsys).
func baselineLines(t *testing.T, fsys fs.FS) []string {
	t.Helper()
	var lines []string

	// Manifest and history files from the catalog root. The golden file uses
	// "manifests/<name>.json" and "manifests/history/<name>.json" paths, so we
	// prepend "manifests/" to match.
	for _, dir := range []string{".", "history"} {
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			if dir == "history" {
				continue // missing history is empty
			}
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if !e.Type().IsRegular() {
				continue
			}
			if !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			path := "manifests/" + e.Name()
			if dir != "." {
				path = "manifests/" + dir + "/" + e.Name()
			}
			readPath := dir + "/" + e.Name()
			if dir == "." {
				readPath = e.Name()
			}
			raw, err := fs.ReadFile(fsys, readPath)
			if err != nil {
				t.Fatalf("read %s: %v", readPath, err)
			}
			sum := sha256.Sum256(raw)
			lines = append(lines, path+" sha256:"+hex.EncodeToString(sum[:]))
		}
	}
	sort.Strings(lines)

	reg, err := BundledFS(fsys)
	if err != nil {
		t.Fatalf("BundledFS: %v", err)
	}
	for _, tmpl := range reg.Held() {
		lines = append(lines, fmt.Sprintf("template %s v%d %s", tmpl.Name, tmpl.Version, tmpl.Hash))
	}

	return lines
}

// The embedded catalog's bytes are pinned so moving it out of the binary cannot change a body, a hash, or the set version unnoticed. Regenerate with -update only when a manifest change is intended.
func TestBaseline(t *testing.T) {
	embedded := embedded()
	lines := baselineLines(t, embedded)
	compareAgainstGolden(t, lines)
}

// TestBaseline_DiskCopy runs the same baseline checks against a copy of the embedded
// catalog on disk, with a .DS_Store and a notes/ directory added, to prove the FS
// functions ignore them.
func TestBaseline_DiskCopy(t *testing.T) {
	dir := t.TempDir()
	copyFS := writeCatalogCopy(t, dir)
	lines := baselineLines(t, copyFS)
	compareAgainstGolden(t, lines)
}

// writeCatalogCopy copies the embedded catalog to dir, adding a .DS_Store and a
// notes/extra.json to prove they are ignored. Returns an fs.FS over dir.
func writeCatalogCopy(t *testing.T, dir string) fs.FS {
	t.Helper()
	root := embedded()
	if err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	}); err != nil {
		t.Fatalf("walk embedded catalog: %v", err)
	}
	// Add a .DS_Store file (non-JSON, should be ignored).
	if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("mock"), 0o644); err != nil {
		t.Fatalf("write .DS_Store: %v", err)
	}
	// Add a notes/ directory with a JSON file (non-regular at root, ignored).
	if err := os.MkdirAll(filepath.Join(dir, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes", "extra.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write notes/extra.json: %v", err)
	}
	return os.DirFS(dir)
}

// compareAgainstGolden compares lines against testdata/baseline.golden, or rewrites
// it when -update is set.
func compareAgainstGolden(t *testing.T, lines []string) {
	t.Helper()
	golden := filepath.Join("testdata", "baseline.golden")
	if *update {
		if err := os.WriteFile(golden, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
			t.Fatalf("write %s: %v", golden, err)
		}
		return
	}
	want := readGolden(t, golden)
	for _, d := range diffLines(lines, want) {
		t.Error(d)
	}
}

// readGolden reads a golden file, split into lines without the trailing newline.
func readGolden(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v (generate it with -update)", path, err)
	}
	text := strings.TrimSuffix(string(raw), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// diffLines reports how the embedded catalog and the golden differ, one message per
// differing line, naming the path or template and both values.
func diffLines(got, want []string) []string {
	var diffs []string
	for i := 0; i < len(got) || i < len(want); i++ {
		var g, w string
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g == w {
			continue
		}
		switch {
		case i >= len(want):
			diffs = append(diffs, fmt.Sprintf("%s is embedded but not in baseline.golden (an added file?)", g))
		case i >= len(got):
			diffs = append(diffs, fmt.Sprintf("%s is in baseline.golden but not embedded (a removed file?)", w))
		default:
			diffs = append(diffs, fmt.Sprintf("line %d differs:\n  embedded: %s\n  golden:   %s", i+1, g, w))
		}
	}
	return diffs
}
