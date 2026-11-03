package catalogtest_test

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// moduleRoot finds go.mod above this file.
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

// modulePath reads the module line of root's go.mod.
func modulePath(root string) (string, error) {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }() // read-only; a close error loses nothing
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(rest), `"`), nil
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no module line in %s", filepath.Join(root, "go.mod"))
}

// testFiles lists every _test.go under root, module-relative, skipping testdata,
// vendor, hidden directories and nested modules, which go test ./... does not build
// as this module's tests either.
func testFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if p != root && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")) {
				return filepath.SkipDir
			}
			if p != root {
				if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if d.Type().IsRegular() && strings.HasSuffix(d.Name(), "_test.go") {
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

// findReaches parses every test file in the module at root and returns each place
// one reads the embedded catalog, and the files it read.
func findReaches(root string) ([]reach, []string, error) {
	mod, err := modulePath(root)
	if err != nil {
		return nil, nil, err
	}
	files, err := testFiles(root)
	if err != nil {
		return nil, nil, err
	}
	var all []reach
	for _, rel := range files {
		rs, err := fileReaches(root, mod, rel)
		if err != nil {
			return nil, nil, err
		}
		all = append(all, rs...)
	}
	return all, files, nil
}
