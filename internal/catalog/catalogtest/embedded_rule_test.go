package catalogtest_test

import (
	"fmt"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// exempt names one test that selects the empty catalog on purpose, and why.
type exempt struct {
	file   string // module-relative
	fn     string // the top-level function
	reason string
}

// emptyOnPurpose names the only tests that may select the empty catalog: catalog.Load
// with an empty directory, or a config catalog directory set to "". Each is listed
// by its function, and is excused only while it passes every catalog it loads, and
// at least as many catalogs as it has selections, to a catalogtest Require helper
// written in its own body (not inside a function literal). Through R4a and R4b (card
// 255) this list named the tests that read the catalog the binary carried; those were
// deleted with it in R4b slice 5 or moved to the deployment's overlay
// (anam_mattjoyce/engine-tests).
var emptyOnPurpose = []exempt{
	{file: "internal/daemon/spatial_test.go", fn: "TestSpatialEmptyCatalogThenRegistration", reason: "spatial selection can start on an empty catalog and draw later registered data without restart"},
	{file: "internal/catalog/empty_test.go", fn: "TestLoadNoDirectoriesIsEmpty",
		reason: "both directories empty is the empty catalog: none, no bodies, the empty V"},
	{file: "internal/catalog/empty_test.go", fn: "TestLoadOneDirectoryEmpty",
		reason: "one directory empty is that component empty beside the other's directory"},
	{file: "internal/catalog/empty_test.go", fn: "TestLoadEmptyIsBuiltOnce",
		reason: "the empty catalog is built once and shared, and a canceled context still refuses it"},
	{file: "internal/catalog/catalogtest/catalogtest_test.go", fn: "TestRequire_PassesOnlyTheEmptyCatalog",
		reason: "the Require helpers' own test needs the empty catalog, and a half-empty one, to pass"},
	{file: "internal/daemon/catalog_start_test.go", fn: "TestOpen_NoCatalogStartsEmpty",
		reason: "a daemon on a config with no catalog directories starts on the empty catalog and logs the WARN and structural lines"},
}

// TestTests_ReadNoEmbeddedCatalog parses every _test.go in the module and fails on
// any selection of the empty catalog outside emptyOnPurpose: catalog.Load (or load
// inside the package) with an empty directory or called through a variable, a config
// catalog directory set to "", or a call to a test helper in the same package that
// does either. It also fails on a listed function that has gone, that no longer
// selects, or that does not check what it selects with a Require helper.
//
// Before R4b slice 5 an empty directory fell back on the catalog the binary carried,
// Matt's, and a test that read it proved things about one person's record; the rule
// then also matched the embedded def, projection and catalog symbols and their source
// directories. The binary carries no catalog now, so those cannot compile, and what an
// empty directory selects is the empty catalog. A test that selects it by mistake,
// meaning a fixture, can pass vacuously over no types; the rule stops that.
//
// A deliberate way round it would show in review. What it cannot see: a config that
// leaves its catalog directories unset rather than setting them to "" (a YAML config
// with no definitions_dir is the same case), which the fixture helpers' own tests
// guard (daemon's TestNewConfig_ReadsTheFixtureCatalog, app's
// TestFixtures_ReadTheFixtureCatalog); an empty directory held in a value it cannot
// resolve in the file (a package-level const in another file, a field, a function's
// result); and a wrapper that reaches from outside the test files, in a non-test .go
// file or through an exported helper of another test package. It matches a method
// call by the method's name, as the parser does not know a receiver's type. For a
// config selection it counts Require calls rather than following the config into the
// daemon, since the daemon's catalog is a field the parser cannot trace.
func TestTests_ReadNoEmbeddedCatalog(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	s, err := findReaches(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.files) < 100 {
		t.Fatalf("read %d test files under %s, want the module's", len(s.files), root)
	}
	for _, p := range checkReaches(s, emptyOnPurpose) {
		t.Error(p)
	}
}

// TestEmptyOnPurpose_EntriesAreWellFormed keeps each entry giving a reason and naming
// a file that exists.
func TestEmptyOnPurpose_EntriesAreWellFormed(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	for _, e := range emptyOnPurpose {
		if e.reason == "" {
			t.Errorf("%s %s has no reason", e.file, e.fn)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(e.file))); err != nil {
			t.Errorf("%s: %v", e.file, err)
		}
	}
}

// listShape returns what is wrong with the list's shape: a function listed twice or
// an entry naming no function.
func listShape(list []exempt) []string {
	var problems []string
	for i, e := range list {
		if e.fn == "" {
			problems = append(problems, fmt.Sprintf("%s: an entry names no function", e.file))
		}
		for _, b := range list[:i] {
			if b.file == e.file && b.fn == e.fn {
				problems = append(problems, fmt.Sprintf("%s %s is listed twice", e.file, e.fn))
			}
		}
	}
	return problems
}

// checkReaches returns what is wrong: the list's shape; each listed function that has
// gone, selects nothing, or does not check what it selects; and each selection outside
// a listed function. A selection spreads to every test function of the same package
// that names the function it sits in, transitively, so calling a listed test or a
// helper that selects is held to the list as well.
func checkReaches(s scan, list []exempt) []string {
	problems := listShape(list)
	type key struct{ file, fn string }
	listed := map[key]bool{}
	for _, e := range list {
		k := key{e.file, e.fn}
		listed[k] = true
		switch {
		case !slices.Contains(s.files, e.file):
			problems = append(problems, fmt.Sprintf("listed %s is not a test file in the module; drop its entry", e.file))
			continue
		case !s.funcs[[2]string{e.file, e.fn}]:
			problems = append(problems, fmt.Sprintf("listed %s %s is not a function in that file; drop its entry", e.file, e.fn))
			continue
		}
		var mine []reach
		lines := map[int]bool{}
		for _, r := range s.reaches {
			if r.File == e.file && r.Func == e.fn {
				mine = append(mine, r)
				lines[r.Line] = true
			}
		}
		var checks []check
		for _, c := range s.checks {
			if c.File == e.file && c.Func == e.fn {
				checks = append(checks, c)
			}
		}
		if len(mine) == 0 {
			problems = append(problems, fmt.Sprintf("listed %s %s no longer selects the empty catalog; drop its entry", e.file, e.fn))
			continue
		}
		if len(checks) < len(lines) {
			problems = append(problems, fmt.Sprintf("listed %s %s selects the empty catalog on %d lines and calls a catalogtest Require helper %d times in its body; check each selection",
				e.file, e.fn, len(lines), len(checks)))
		}
		for _, r := range mine {
			if r.Kind == kindLoad && !passed(r.Bind, checks) {
				problems = append(problems, fmt.Sprintf("%s: listed, but the catalog this load returns is not passed to a catalogtest Require helper in the function's body", r))
			}
		}
	}
	for _, r := range append(slices.Clone(s.reaches), throughHelpers(s.reaches, s.refs)...) {
		if r.Kind != kindHelper && listed[key{r.File, r.Func}] {
			continue
		}
		problems = append(problems, fmt.Sprintf("%s selects the empty catalog outside emptyOnPurpose; "+
			"read a fixture (catalogtest.Engine, fixture.Minimal), or list the test and check what it selects", r))
	}
	return problems
}

// passed reports whether the variable declared at bind is an argument of one of checks.
func passed(bind token.Pos, checks []check) bool {
	if bind == token.NoPos {
		return false
	}
	for _, c := range checks {
		if slices.Contains(c.Args, bind) {
			return true
		}
	}
	return false
}

// throughHelpers returns a reach for each function that names, in the same package,
// a function that reaches, until no more do. A method is named by "*.m" for any
// receiver.
func throughHelpers(direct []reach, refs []ref) []reach {
	type fn struct{ pkg, name string }
	byName := map[fn][]ref{}
	for _, r := range refs {
		byName[fn{r.Pkg, r.To}] = append(byName[fn{r.Pkg, r.To}], r)
	}
	reaching := map[fn]bool{}
	var queue []fn
	mark := func(f fn) {
		if f.name != "" && !reaching[f] {
			reaching[f] = true
			queue = append(queue, f)
		}
	}
	for _, r := range direct {
		mark(fn{r.Pkg, r.Func})
	}
	var found []reach
	seen := map[[3]string]bool{}
	for len(queue) > 0 {
		h := queue[0]
		queue = queue[1:]
		users := byName[h]
		if _, m, ok := strings.Cut(h.name, "."); ok {
			users = append(slices.Clone(users), byName[fn{h.pkg, "*." + m}]...)
		}
		for _, r := range users {
			k := [3]string{r.Pkg, r.From, h.name}
			if r.From == h.name || seen[k] {
				continue
			}
			seen[k] = true
			found = append(found, reach{Pkg: r.Pkg, File: r.File, Func: r.From, Line: r.Line, Kind: kindHelper,
				What: "via helper " + h.name})
			mark(fn{r.Pkg, r.From})
		}
	}
	return found
}
