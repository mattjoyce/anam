package fixture_test

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/fixture"
)

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

// TestDirs_WritesTheSameBytes checks Dirs writes every file of both roots unchanged,
// history/ included.
func TestDirs_WritesTheSameBytes(t *testing.T) {
	t.Parallel()
	defs, projs := fixture.Engine()
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
