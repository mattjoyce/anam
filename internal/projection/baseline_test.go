package projection

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// update rewrites the golden file instead of comparing, for a deliberate catalog
// change.
var update = flag.Bool("update", false, "rewrite testdata/baseline.golden instead of comparing")

// The embedded catalog's bytes are pinned so moving it out of the binary cannot change a body, a hash, or the set version unnoticed. Regenerate with -update only when a manifest change is intended.
func TestBaseline(t *testing.T) {
	var lines []string
	for _, dir := range []string{"manifests", "manifests/history"} {
		entries, err := manifestFS.ReadDir(dir)
		if err != nil {
			t.Fatalf("read embedded %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := dir + "/" + e.Name()
			raw, err := manifestFS.ReadFile(path)
			if err != nil {
				t.Fatalf("read embedded %s: %v", path, err)
			}
			sum := sha256.Sum256(raw)
			lines = append(lines, path+" sha256:"+hex.EncodeToString(sum[:]))
		}
	}
	sort.Strings(lines)

	reg, err := Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	for _, tmpl := range reg.Held() {
		lines = append(lines, fmt.Sprintf("template %s v%d %s", tmpl.Name, tmpl.Version, tmpl.Hash))
	}

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
