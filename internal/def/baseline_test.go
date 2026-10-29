package def

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
// returns the manifest lines; it also reads the sourcetime files from sourceTimeFS,
// and appends the set_version computed from fsys. The returned lines are in the same
// format as the golden file.
func baselineLines(fsys fs.FS) []string {
	var lines []string

	// Manifest and history files from the catalog root. The golden file uses
	// "manifests/<name>.json" and "manifests/history/<name>.json" paths, so we
	// prepend "manifests/" to match.
	for _, dir := range []string{".", "history"} {
		entries, err := fs.ReadDir(fsys, dir)
		if err != nil {
			// A missing history/ is empty, not an error.
			if dir == "history" {
				continue
			}
			panic("baselineLines: read " + dir + ": " + err.Error())
		}
		for _, e := range entries {
			if !e.Type().IsRegular() {
				continue
			}
			if !isJSON(e.Name()) {
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
				panic("baselineLines: read " + readPath + ": " + err.Error())
			}
			sum := sha256.Sum256(raw)
			lines = append(lines, path+" sha256:"+hex.EncodeToString(sum[:]))
		}
	}
	sort.Strings(lines)

	// Sourcetime files are read from sourceTimeFS directly.
	var stLines []string
	entries, err := sourceTimeFS.ReadDir("sourcetime")
	if err != nil {
		panic("baselineLines: read sourcetime: " + err.Error())
	}
	for _, e := range entries {
		raw, err := sourceTimeFS.ReadFile("sourcetime/" + e.Name())
		if err != nil {
			panic("baselineLines: read sourcetime/" + e.Name() + ": " + err.Error())
		}
		sum := sha256.Sum256(raw)
		stLines = append(stLines, "sourcetime/"+e.Name()+" sha256:"+hex.EncodeToString(sum[:]))
	}
	sort.Strings(stLines)
	lines = append(lines, stLines...)

	sv := setVersionOfFS(fsys)
	lines = append(lines, "set_version "+sv)
	return lines
}

// setVersionOfFS computes the set version of a catalog root: content-address of every
// *.json at the root.
func setVersionOfFS(fsys fs.FS) string {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return ""
	}
	files := make(map[string][]byte)
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if !isJSON(e.Name()) {
			continue
		}
		raw, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			continue
		}
		files[e.Name()] = raw
	}
	return setVersionOf(files)
}

// The embedded catalog's bytes are pinned so moving it out of the binary cannot change a body, a hash, or the set version unnoticed. Regenerate with -update only when a manifest change is intended.
func TestBaseline(t *testing.T) {
	embedded := embedded()
	lines := baselineLines(embedded)
	compareAgainstGolden(t, lines)
}

// TestBaseline_DiskCopy runs the same baseline checks against a copy of the embedded
// catalog on disk, with a .DS_Store and a notes/ directory added, to prove the FS
// functions ignore them.
func TestBaseline_DiskCopy(t *testing.T) {
	dir := t.TempDir()
	copyFS := writeCatalogCopy(t, dir)
	lines := baselineLines(copyFS)
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
// differing line, naming the path (or set_version) and both values.
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
