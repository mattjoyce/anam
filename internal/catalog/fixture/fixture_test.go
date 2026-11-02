package fixture_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/fixture"
)

// countJSON is how many *.json files a root holds at its top and under history/.
func countJSON(t *testing.T, root fs.FS) (current, history int) {
	t.Helper()
	for dir, n := range map[string]*int{".": &current, "history": &history} {
		names, err := fs.Glob(root, filepath.ToSlash(filepath.Join(dir, "*.json")))
		if err != nil {
			t.Fatal(err)
		}
		*n = len(names)
	}
	return current, history
}

// TestMattjoyce_Layout pins the copy's shape: 44 current and 90 earlier
// Definitions, 12 current and 6 earlier templates, as embedded at de2981c.
func TestMattjoyce_Layout(t *testing.T) {
	t.Parallel()
	defs, projs := fixture.Mattjoyce()
	if c, h := countJSON(t, defs); c != 44 || h != 90 {
		t.Errorf("definitions: %d current, %d history; want 44, 90", c, h)
	}
	if c, h := countJSON(t, projs); c != 12 || h != 6 {
		t.Errorf("projections: %d current, %d history; want 12, 6", c, h)
	}
}

// TestMinimal_Layout pins what the minimal catalog holds.
func TestMinimal_Layout(t *testing.T) {
	t.Parallel()
	defs, projs := fixture.Minimal()
	for root, want := range map[fs.FS][]string{
		defs:  {"record.document.json", "record.edge.json", "test.measure.json"},
		projs: {"measures.json"},
	} {
		got, err := fs.Glob(root, "*")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != len(want) {
			t.Errorf("files = %v, want %v", got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("files = %v, want %v", got, want)
				break
			}
		}
	}
}

// TestDirs_WritesTheSameBytes checks Dirs writes every file of both roots unchanged.
func TestDirs_WritesTheSameBytes(t *testing.T) {
	t.Parallel()
	defs, projs := fixture.Mattjoyce()
	defsDir, projsDir := fixture.Dirs(t, defs, projs)
	for dir, root := range map[string]fs.FS{defsDir: defs, projsDir: projs} {
		n := 0
		err := fs.WalkDir(root, ".", func(name string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			want, err := fs.ReadFile(root, name)
			if err != nil {
				return err
			}
			got, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
			if err != nil {
				return err
			}
			if !bytes.Equal(got, want) {
				t.Errorf("%s/%s differs from the fixture", dir, name)
			}
			n++
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Errorf("%s: nothing written", dir)
		}
	}
}
