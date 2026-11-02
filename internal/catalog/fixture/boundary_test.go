package fixture_test

import (
	"errors"
	"go/build"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
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

// TestBoundary_ProductionDoesNotReachFixtures fails when a production package can
// carry a test catalog. A binary that carried one would carry a catalog nobody chose.
// It fails on a production package that reaches fixture or catalogtest through its
// non-test imports, directly or through any other package (one under testdata
// included, which ./... leaves out but an explicit import reaches); that builds on a
// package whose //go:embed names fixture's testdata; or that builds on a package with
// native sources (cgo, assembly, .syso), which can carry bytes with no import or embed.
// It also fails on a symlink to a directory, or into the fixture, anywhere in the
// module, since an alias directory is an import path the graph cannot see through.
//
// It reads the source with go/build and runs no go command, so it is hermetic.
// scripts/check.sh holds the same rules over go list, so deleting this test does not
// lift them. The guards stop a test fixture being relied on by accident; a deliberate
// way round them would show in review.
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

// TestBoundary_SeesWhatReachesTheFixture runs the walk over small modules holding
// each way in, and each case that is allowed, so a clean pass is not a walk that read
// nothing.
func TestBoundary_SeesWhatReachesTheFixture(t *testing.T) {
	t.Parallel()
	imp := func(pkg, path string) string {
		return "package " + pkg + "\n\nimport _ \"" + module + "/" + path + "\"\n"
	}
	embed := func(pkg, pattern string) string {
		return "package " + pkg + "\n\nimport \"embed\"\n\n//go:embed " + pattern + "\nvar files embed.FS\n"
	}
	for _, c := range []struct {
		name  string
		files map[string]string // a value starting "-> " is a symlink to the rest
		want  []string          // offending directories, or "symlink <path>"
	}{
		{name: "clean", want: nil},
		{name: "direct import", files: map[string]string{
			"internal/direct/direct.go": imp("direct", fixtureDir),
		}, want: []string{"internal/direct"}},
		{name: "through a helper on catalogtest", files: map[string]string{
			"internal/helper/helper.go": "package helper\n\nimport (\n\t\"fmt\"\n\tct \"" + module + "/" + catalogtestDir + "\"\n)\n",
			"internal/above/above.go":   imp("above", "internal/helper"),
		}, want: []string{"internal/above", "internal/helper"}},
		{name: "through a package under testdata", files: map[string]string{
			"internal/testdata/bridge/bridge.go": imp("bridge", fixtureDir),
			"cmd/tool/main.go":                   imp("main", "internal/testdata/bridge"),
		}, want: []string{"cmd/tool"}},
		{name: "embed by a relative path", files: map[string]string{
			"internal/catalog/embeds.go": embed("catalog", "fixture/testdata/minimal"),
			"cmd/tool/main.go":           imp("main", "internal/catalog"),
		}, want: []string{"cmd/tool", "internal/catalog"}},
		{name: "embed of a directory that holds it", files: map[string]string{
			"internal/whole.go": embed("internal", "`catalog`"),
		}, want: []string{"internal"}},
		{name: "symlinked directory alias", files: map[string]string{
			"internal/fixturealias": "-> catalog/fixture",
			"cmd/tool/main.go":      imp("main", "internal/fixturealias"),
		}, want: []string{"symlink internal/fixturealias"}},
		{name: "symlinked file into the fixture", files: map[string]string{
			"internal/catalog/copy.go": "-> fixture/fixture.go",
		}, want: []string{"symlink internal/catalog/copy.go"}},
		{name: "assembly", files: map[string]string{
			"internal/native/native.go":       "package native\n",
			"internal/native/native_amd64.s":  "#include \"textflag.h\"\n",
			"internal/native/native_arm64.s":  "#include \"textflag.h\"\n",
			"internal/native/native_other.go": "package native\n",
			"cmd/tool/main.go":                imp("main", "internal/native"),
		}, want: []string{"cmd/tool", "internal/native"}},
		{name: "cgo", files: map[string]string{
			"internal/catalog/cgo.go": "package catalog\n\n/*\n__asm__(\".incbin \\\"fixture/testdata/minimal/x.json\\\"\");\n*/\nimport \"C\"\n",
		}, want: []string{"internal/catalog"}},
		{name: "allowed", files: map[string]string{
			"internal/clean/clean.go":                         "package clean\n",
			"internal/clean/clean_test.go":                    imp("clean", fixtureDir),
			"internal/clean/testdata/fixture/testdata/x.json": "{}",
			"internal/own/own.go":                             embed("own", "testdata"),
			"internal/own/testdata/y.json":                    "{}",
			"internal/testdata/unused/unused.go":              imp("unused", fixtureDir),
			"internal/testdata/notes.go.txt":                  imp("notes", fixtureDir),
			"internal/notes.md":                               "-> ../README.md",
			"README.md":                                       "words\n",
			// Directive text that is not a directive: in a string, in a block comment.
			"internal/catalog/strings.go": "package catalog\n\nimport \"embed\"\n\nvar _ embed.FS\n\n" +
				"const s = `\n//go:embed fixture/testdata/minimal\n`\n",
			"internal/catalog/comment.go": "package catalog\n\nimport \"embed\"\n\n/*\n//go:embed fixture/testdata/minimal\n*/\n" +
				"var t embed.FS\n",
		}, want: nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			files := map[string]string{
				"go.mod":                           "module " + module + "\n",
				fixtureData + "/minimal/x.json":    "{}",
				fixtureDir + "/fixture.go":         embed("fixture", "testdata/minimal"),
				catalogtestDir + "/catalogtest.go": imp("catalogtest", fixtureDir),
			}
			for name, body := range c.files {
				files[name] = body
			}
			for name, body := range files {
				p := filepath.Join(dir, filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
					t.Fatal(err)
				}
				if target, ok := strings.CutPrefix(body, "-> "); ok {
					err := os.Symlink(filepath.FromSlash(target), p)
					if err != nil {
						t.Fatal(err)
					}
					continue
				}
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := productionReaching(dir)
			if err != nil {
				t.Fatal(err)
			}
			var offenders []string
			for _, o := range got {
				offenders = append(offenders, strings.SplitN(o, " (", 2)[0])
			}
			if strings.Join(offenders, ",") != strings.Join(c.want, ",") {
				t.Errorf("offenders = %q\nwant %q", got, c.want)
			}
		})
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

// pkg is what the walk learns of one directory's non-test Go package.
type pkg struct {
	imports []string // module-relative directories this package imports
	carries string   // why the package itself carries fixture bytes, or ""
}

// productionReaching reads every package under root, testdata included, builds the
// module's import graph, and returns each production package (outside testdata,
// other than fixture and catalogtest) that reaches a carrier, with the path it takes
// and why the carrier carries, then every symlink the walk refuses. Sorted.
func productionReaching(root string) ([]string, error) {
	pkgs, links, err := readPackages(root)
	if err != nil {
		return nil, err
	}
	carrier := func(dir string) string {
		switch {
		case dir == fixtureDir:
			return "is fixture"
		case dir == catalogtestDir:
			return "is catalogtest"
		case pkgs[dir] != nil:
			return pkgs[dir].carries
		}
		return ""
	}
	var offenders []string
	for dir := range pkgs {
		if dir == fixtureDir || dir == catalogtestDir || inTestdata(dir) {
			continue
		}
		if via, why := reach(pkgs, dir, carrier); via != nil {
			offenders = append(offenders, dir+" (via "+strings.Join(via, " -> ")+", which "+why+")")
		}
	}
	sort.Strings(offenders)
	sort.Strings(links)
	return append(offenders, links...), nil
}

// reach is the import path from start to the first carrier it reaches, breadth
// first, and why that one carries; nil when it reaches none.
func reach(pkgs map[string]*pkg, start string, carrier func(string) string) ([]string, string) {
	parent := map[string]string{start: ""}
	queue := []string{start}
	for len(queue) > 0 {
		dir := queue[0]
		queue = queue[1:]
		if why := carrier(dir); why != "" {
			var via []string
			for d := dir; d != ""; d = parent[d] {
				via = append([]string{d}, via...)
			}
			return via, why
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
	return nil, ""
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

// readPackages walks root, skipping dot directories and nested modules, and reads
// each directory's package with go/build: its non-test imports, its //go:embed
// patterns as the compiler sees them, and its native sources. It also returns every
// symlink that resolves to a directory or into the fixture, as "symlink <path>".
func readPackages(root string) (map[string]*pkg, []string, error) {
	ctx := build.Default
	ctx.CgoEnabled = true // so a cgo file is read as one, not skipped
	realFixture, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(fixtureDir)))
	if err != nil {
		return nil, nil, err
	}
	pkgs := map[string]*pkg{}
	var links []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if d.Type()&fs.ModeSymlink != 0 {
			bad, err := refusedLink(p, realFixture)
			if err != nil {
				return err
			}
			if bad {
				links = append(links, "symlink "+rel)
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if p != root {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(p, "go.mod")); err == nil {
				return filepath.SkipDir // another module
			}
		}
		bp, err := ctx.ImportDir(p, 0)
		var noGo *build.NoGoError
		if errors.As(err, &noGo) {
			return nil
		}
		if err != nil {
			return err
		}
		pk := &pkg{}
		for _, ip := range bp.Imports {
			if dep, ok := strings.CutPrefix(ip, module+"/"); ok {
				pk.imports = append(pk.imports, dep)
			}
		}
		if native := nativeFiles(bp); len(native) > 0 {
			pk.carries = "has native sources " + strings.Join(native, " ")
		}
		if rel != fixtureDir {
			hit, err := embedsFixture(root, rel, bp.EmbedPatterns)
			if err != nil {
				return err
			}
			if hit {
				pk.carries = "embeds fixture testdata"
			}
		}
		pkgs[rel] = pk
		return nil
	})
	return pkgs, links, err
}

// nativeFiles is every non-Go source a package builds with. cgo and assembly can
// carry bytes (an .incbin, a .syso) that no import or //go:embed shows, and the
// module has none, so any is refused rather than read.
func nativeFiles(bp *build.Package) []string {
	var out []string
	for _, files := range [][]string{
		bp.CgoFiles, bp.CFiles, bp.CXXFiles, bp.MFiles, bp.HFiles, bp.FFiles,
		bp.SFiles, bp.SwigFiles, bp.SwigCXXFiles, bp.SysoFiles,
	} {
		out = append(out, files...)
	}
	return out
}

// refusedLink reports whether a symlink resolves to a directory, which would be an
// import path the walk does not follow, or to anything inside the fixture.
func refusedLink(link, realFixture string) (bool, error) {
	target, err := filepath.EvalSymlinks(link)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil // dangling: carries nothing
	}
	if err != nil {
		return false, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return false, err
	}
	rel, err := filepath.Rel(realFixture, target)
	if err != nil {
		return false, err
	}
	inside := rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
	return info.IsDir() || inside, nil
}

// embedsFixture reports whether an //go:embed pattern of the package in the
// module-relative directory dir names a file of fixture's testdata, or a directory
// that holds it.
func embedsFixture(root, dir string, patterns []string) (bool, error) {
	for _, pat := range patterns {
		pat = strings.TrimPrefix(pat, "all:")
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(dir), filepath.FromSlash(pat)))
		if err != nil {
			return false, err
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
	return false, nil
}

// within reports whether slash path a is b or below it.
func within(a, b string) bool {
	a, b = path.Clean(a), path.Clean(b)
	return a == b || strings.HasPrefix(a, b+"/")
}
