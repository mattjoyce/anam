package fixture_test

import (
	"errors"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The test-only packages, and the directories that are allowed to import them
// outside a _test.go file: their own.
const (
	module         = "github.com/mattjoyce/anam"
	fixturePkg     = module + "/internal/catalog/fixture"
	catalogtestPkg = module + "/internal/catalog/catalogtest"
)

// TestBoundary_ProductionDoesNotImportFixtures fails on any non-test Go file in the
// module that imports fixture or catalogtest, other than catalogtest itself. A binary
// that carried a fixture would carry a catalog nobody chose. It parses imports only
// and runs no go command, so it is hermetic. scripts/check.sh holds the same rule
// over go list, so deleting this test does not lift it.
func TestBoundary_ProductionDoesNotImportFixtures(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	offenders, err := productionImporters(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range offenders {
		t.Errorf("%s imports a test fixture outside a test", o)
	}
}

// TestBoundary_SeesAnImport checks the walk finds an import where there is one, so a
// clean pass is not a walk that read nothing.
func TestBoundary_SeesAnImport(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module "+module+"\n")
	write("internal/app/run.go", "package app\n\nimport _ \""+fixturePkg+"\"\n")
	write("internal/app/run_test.go", "package app\n\nimport _ \""+fixturePkg+"\"\n")
	write("internal/serve/serve.go", "package serve\n\nimport (\n\t\"fmt\"\n\tct \""+catalogtestPkg+"\"\n)\n")
	write("internal/catalog/catalogtest/catalogtest.go", "package catalogtest\n\nimport _ \""+fixturePkg+"\"\n")
	write("internal/def/testdata/x.go", "package x\n\nimport _ \""+fixturePkg+"\"\n")

	got, err := productionImporters(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"internal/app/run.go", "internal/serve/serve.go"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("offenders = %v, want %v", got, want)
	}
}

// moduleRoot is the nearest directory above this file that holds go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate this test file")
	}
	for dir := filepath.Dir(file); ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		} else if !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		if parent := filepath.Dir(dir); parent == dir {
			t.Fatalf("no go.mod above %s", file)
		}
	}
}

// productionImporters walks root as the go tool would (skipping testdata, vendor and
// directories starting with . or _) and returns, relative to root and in walk order,
// every non-test .go file that imports fixture or catalogtest, outside catalogtest.
func productionImporters(root string) ([]string, error) {
	var offenders []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && (name == "testdata" || name == "vendor" ||
				strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, "internal/catalog/catalogtest/") {
			return nil // catalogtest is built on fixture, and is itself test-only
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if p == fixturePkg || p == catalogtestPkg {
				offenders = append(offenders, rel)
				break
			}
		}
		return nil
	})
	return offenders, err
}
