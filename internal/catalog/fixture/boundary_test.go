package fixture_test

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The test-only packages, and where the fixture's bytes live.
const (
	module         = "github.com/mattjoyce/anam"
	fixtureDir     = "internal/catalog/fixture"
	catalogtestDir = "internal/catalog/catalogtest"
	fixtureData    = fixtureDir + "/testdata"
)

// TestBoundary_ProductionDoesNotReachFixtures fails on any production package in the
// module that reaches fixture or catalogtest through its non-test imports, directly
// or through any other package (one under testdata included, which ./... leaves out
// but an explicit import reaches), or that builds on a package that //go:embeds a
// file of fixture's testdata. A binary that carried a fixture would carry a catalog
// nobody chose. It parses source and runs no go command, so it is hermetic.
// scripts/check.sh holds the same rule over go list, so deleting this test does not
// lift it.
func TestBoundary_ProductionDoesNotReachFixtures(t *testing.T) {
	t.Parallel()
	offenders, err := productionReaching(moduleRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range offenders {
		t.Errorf("%s carries a test catalog outside a test", o)
	}
}

// TestBoundary_SeesWhatReachesTheFixture runs the walk over a small module holding
// every way in, and every way that is allowed, so a clean pass is not a walk that
// read nothing.
func TestBoundary_SeesWhatReachesTheFixture(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	imp := func(pkg, path string) string {
		return "package " + pkg + "\n\nimport _ \"" + module + "/" + path + "\"\n"
	}
	embed := func(pkg, pattern string) string {
		return "package " + pkg + "\n\nimport \"embed\"\n\n//go:embed " + pattern + "\nvar files embed.FS\n"
	}
	write("go.mod", "module "+module+"\n")
	write(fixtureData+"/minimal/definitions/x.json", "{}")
	write(fixtureDir+"/fixture.go", embed("fixture", "testdata/minimal/definitions"))
	write(catalogtestDir+"/catalogtest.go", imp("catalogtest", fixtureDir))

	// Ways in.
	write("internal/direct/direct.go", imp("direct", fixtureDir))
	write("internal/helper/helper.go", "package helper\n\nimport (\n\t\"fmt\"\n\tct \""+module+"/"+catalogtestDir+"\"\n)\n")
	write("internal/above/above.go", imp("above", "internal/helper"))
	write("internal/testdata/bridge/bridge.go", imp("bridge", fixtureDir))
	write("internal/viabridge/viabridge.go", imp("viabridge", "internal/testdata/bridge"))
	write("internal/catalog/embeds.go", embed("catalog", "fixture/testdata/minimal/definitions"))
	write("internal/star/star.go", "package star\n")
	write("internal/star/fixture/testdata/x.json", "{}")
	write("internal/whole.go", embed("internal", "`catalog`"))
	write("internal/testdata/embedder/embedder.go", embed("embedder", "\"x/*.json\""))
	write("internal/testdata/embedder/x/a.json", "{}")
	write("cmd/tool/main.go", imp("main", "internal/catalog"))

	// Allowed.
	write("internal/clean/clean.go", "package clean\n")
	write("internal/clean/clean_test.go", imp("clean", fixtureDir))
	write("internal/clean/testdata/fixture/testdata/x.json", "{}")
	write("internal/own/own.go", embed("own", "testdata"))
	write("internal/own/testdata/y.json", "{}")
	write("internal/testdata/unused/unused.go", imp("unused", fixtureDir))
	write("internal/testdata/notes.go.txt", imp("notes", fixtureDir))

	got, err := productionReaching(dir)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, o := range got {
		dirs = append(dirs, strings.Fields(o)[0])
	}
	want := []string{
		"cmd/tool",           // through internal/catalog's embed
		"internal",           // embeds a directory that holds fixture's testdata
		"internal/above",     // through helper
		"internal/catalog",   // embeds fixture testdata by a relative path
		"internal/direct",    // imports fixture
		"internal/helper",    // imports catalogtest
		"internal/viabridge", // through a package under testdata
	}
	if strings.Join(dirs, ",") != strings.Join(want, ",") {
		t.Errorf("offenders = %v\nwant %v", got, want)
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

// pkg is what the walk learns of one directory's non-test Go files.
type pkg struct {
	imports []string // module-relative directories this package imports
	embeds  bool     // a //go:embed pattern reaches fixture's testdata
}

// productionReaching reads every non-test Go file under root, testdata included,
// builds the module's import graph, and returns each production package (outside
// testdata, other than fixture and catalogtest) that reaches a carrier: fixture,
// catalogtest, or a package that embeds fixture's testdata. Each entry is the
// package's directory and the path it reaches the carrier by, sorted.
func productionReaching(root string) ([]string, error) {
	pkgs, err := readPackages(root)
	if err != nil {
		return nil, err
	}
	carrier := func(dir string) bool {
		return dir == fixtureDir || dir == catalogtestDir || (pkgs[dir] != nil && pkgs[dir].embeds)
	}
	var offenders []string
	for dir := range pkgs {
		if dir == fixtureDir || dir == catalogtestDir || inTestdata(dir) {
			continue
		}
		if via := reach(pkgs, dir, carrier); via != nil {
			offenders = append(offenders, dir+" (via "+strings.Join(via, " -> ")+")")
		}
	}
	sort.Strings(offenders)
	return offenders, nil
}

// reach is the import path from start to the first carrier it reaches, breadth
// first, or nil when it reaches none.
func reach(pkgs map[string]*pkg, start string, carrier func(string) bool) []string {
	parent := map[string]string{start: ""}
	queue := []string{start}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		if carrier(dir) {
			var via []string
			for d := dir; d != ""; d = parent[d] {
				via = append([]string{d}, via...)
			}
			return via
		}
		if p := pkgs[dir]; p != nil {
			for _, next := range p.imports {
				if _, seen := parent[next]; !seen {
					parent[next] = dir
					queue = append(queue, next)
				}
			}
		}
	}
	return nil
}

// inTestdata reports whether a module-relative directory is under a testdata
// directory, which the go tool leaves out of ./... .
func inTestdata(dir string) bool {
	for _, e := range strings.Split(dir, "/") {
		if e == "testdata" {
			return true
		}
	}
	return false
}

// readPackages walks root, skipping .git, dot directories and nested modules, and
// reads every non-test .go file's imports and //go:embed patterns, by directory.
func readPackages(root string) (map[string]*pkg, error) {
	pkgs := map[string]*pkg{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p == root {
				return nil
			}
			if strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir // another module
			}
			return nil
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(p))
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		pk := pkgs[rel]
		if pk == nil {
			pk = &pkg{}
			pkgs[rel] = pk
		}
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range f.Imports {
			ip, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if strings.HasPrefix(ip, module+"/") {
				pk.imports = append(pk.imports, strings.TrimPrefix(ip, module+"/"))
			}
		}
		hit, err := embedsFixture(root, rel, p)
		if err != nil {
			return err
		}
		pk.embeds = pk.embeds || (hit && rel != fixtureDir)
		return nil
	})
	return pkgs, err
}

// embedsFixture reports whether a //go:embed directive in file, in the module-relative
// directory dir, names a file of fixture's testdata, or a directory that holds it.
func embedsFixture(root, dir, file string) (bool, error) {
	src, err := os.ReadFile(file) // a file of this module, found by the walk
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(src), "\n") {
		rest, ok := strings.CutPrefix(strings.TrimSpace(line), "//go:embed")
		if !ok {
			continue
		}
		patterns, err := embedPatterns(rest)
		if err != nil {
			return false, fmt.Errorf("%s: %w", file, err)
		}
		for _, pat := range patterns {
			pat = strings.TrimPrefix(pat, "all:")
			matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), filepath.FromSlash(pat)))
			if err != nil {
				return false, fmt.Errorf("%s: pattern %q: %w", file, pat, err)
			}
			for _, m := range matches {
				rel, err := filepath.Rel(root, m)
				if err != nil {
					return false, err
				}
				rel = filepath.ToSlash(rel)
				if within(rel, fixtureData) || within(fixtureData, rel) {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

// within reports whether slash path a is b or below it.
func within(a, b string) bool {
	a, b = path.Clean(a), path.Clean(b)
	return a == b || strings.HasPrefix(a, b+"/")
}

// embedPatterns splits a //go:embed line's arguments: space-separated, each bare or
// quoted as a Go string literal.
func embedPatterns(s string) ([]string, error) {
	var out []string
	for s = strings.TrimSpace(s); s != ""; s = strings.TrimSpace(s) {
		end := strings.IndexAny(s, " \t")
		if q := s[0]; q == '"' || q == '`' {
			end = -1
			for i := 1; i < len(s); i++ {
				if q == '"' && s[i] == '\\' {
					i++
					continue
				}
				if s[i] == q {
					end = i + 1
					break
				}
			}
			if end < 0 {
				return nil, fmt.Errorf("unterminated pattern in %q", s)
			}
			p, err := strconv.Unquote(s[:end])
			if err != nil {
				return nil, fmt.Errorf("pattern %s: %w", s[:end], err)
			}
			out = append(out, p)
			s = s[end:]
			continue
		}
		if end < 0 {
			end = len(s)
		}
		out = append(out, s[:end])
		s = s[end:]
	}
	return out, nil
}
