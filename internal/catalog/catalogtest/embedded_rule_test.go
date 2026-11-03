package catalogtest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// allowed names test code that may select the empty catalog without checking it did,
// and why.
type allowed struct {
	file   string   // module-relative
	funcs  []string // the top-level functions allowed; nil allows the whole file
	reason string
}

// embeddedAllowList is every test excused from the rule. It is nil. Through R4a and
// R4b (card 255) it named the tests that read the catalog the binary carried, whose
// subject was that catalog: the fixture cross-checks, the drift report, the corpus
// and marks coverage tests. Each was deleted with the embedded catalog in R4b slice 5
// or moved to the deployment's overlay (anam_mattjoyce/engine-tests). Slice 6 removed
// the last two, catalog/empty_test.go and daemon's TestOpen_NoCatalogStartsEmpty:
// they select the empty catalog on purpose and check its none label, which the rule
// now reads as the sign of a deliberate selection. An entry added here is a reviewed
// exception and needs its reason.
var embeddedAllowList []allowed

// TestTests_ReadNoEmbeddedCatalog parses every _test.go in the module and fails on
// any test that selects the empty catalog without checking it did, outside
// embeddedAllowList: catalog.Load with an empty directory or called through a
// variable, a config catalog directory set to "", or a call to a test helper in the
// same package that does either. A top-level function that names the label "none",
// the source the empty catalog reports, checks what it selected and is not a reach.
// It also fails on an entry that no longer reaches or names a file that has gone, so
// the list cannot outlive what it allows.
//
// Before R4b slice 5 an empty directory fell back on the catalog the binary carried,
// Matt's, and a test that read it proved things about one person's record; the rule
// then also matched the embedded def, projection and catalog symbols and their
// source directories. The binary carries no catalog now, so those cannot compile, and
// what an empty directory selects is the empty catalog. A test that selects it by
// mistake, meaning a fixture, can pass vacuously over no types; the rule stops that.
//
// A deliberate way round it would show in review. What it cannot see: a config that
// leaves its catalog directories unset rather than setting them to "" (a YAML config
// with no definitions_dir is the same case), which the fixture helpers' own tests
// guard (daemon's TestNewConfig_ReadsTheFixtureCatalog, app's
// TestFixtures_ReadTheFixtureCatalog); an empty directory held in a value it cannot
// resolve in the file (a package-level const in another file, a field, a function's
// result); and a wrapper that reaches from outside the test files, in a non-test .go
// file or through an exported helper of another test package. It matches a method
// call by the method's name, as the parser does not know a receiver's type.
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
	if embeddedAllowList != nil {
		t.Errorf("embeddedAllowList holds %d entries; R4b slice 6 left it nil", len(embeddedAllowList))
	}
	for _, p := range checkReaches(s, embeddedAllowList) {
		t.Error(p)
	}
}

// TestAllowList_EntriesAreWellFormed keeps each entry giving a reason and naming a
// file that exists.
func TestAllowList_EntriesAreWellFormed(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	for _, a := range embeddedAllowList {
		if a.reason == "" {
			t.Errorf("%s has no reason", a.file)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(a.file))); err != nil {
			t.Errorf("%s: %v", a.file, err)
		}
	}
}

// listShape returns what is wrong with the list's shape: a file listed twice, an
// empty function set, and a function listed twice.
func listShape(allow []allowed) []string {
	var problems []string
	for i, a := range allow {
		for _, b := range allow[:i] {
			if b.file == a.file {
				problems = append(problems, fmt.Sprintf("%s is listed twice", a.file))
			}
		}
		if a.funcs != nil && len(a.funcs) == 0 {
			problems = append(problems, fmt.Sprintf("%s lists an empty set of functions; use nil for the whole file", a.file))
		}
		for j, f := range a.funcs {
			if slices.Contains(a.funcs[:j], f) {
				problems = append(problems, fmt.Sprintf("%s lists %s twice", a.file, f))
			}
		}
	}
	return problems
}

// checkReaches returns what is wrong: the list's shape, each reach that no entry
// allows, and each entry (or listed function) that allows nothing or names a file the
// walk did not read. A reach spreads to every test function of the same package that
// names the function it sits in, transitively, so a test calling an allowed helper is
// held to the list as well.
func checkReaches(s scan, allow []allowed) []string {
	problems := listShape(allow)
	type key struct{ file, fn string }
	used := map[key]bool{}
	match := func(r reach) bool {
		for _, a := range allow {
			if a.file != r.File {
				continue
			}
			if a.funcs == nil {
				used[key{a.file, ""}] = true
				return true
			}
			if slices.Contains(a.funcs, r.Func) {
				used[key{a.file, r.Func}] = true
				return true
			}
		}
		return false
	}
	reaches := append(slices.Clone(s.reaches), throughHelpers(s.reaches, s.refs)...)
	for _, r := range reaches {
		if !match(r) {
			problems = append(problems, fmt.Sprintf("%s selects the empty catalog without checking its %s label; "+
				"read a fixture (catalogtest.Engine, fixture.Minimal), or check the label if the empty catalog is the subject", r, noneLabel))
		}
	}
	for _, a := range allow {
		if !slices.Contains(s.files, a.file) {
			problems = append(problems, fmt.Sprintf("listed %s is not a test file in the module; drop its entry", a.file))
			continue
		}
		if a.funcs == nil {
			if !used[key{a.file, ""}] {
				problems = append(problems, fmt.Sprintf("listed %s no longer reaches; drop its entry", a.file))
			}
			continue
		}
		for _, f := range a.funcs {
			if !used[key{a.file, f}] {
				problems = append(problems, fmt.Sprintf("listed %s %s no longer reaches; drop it", a.file, f))
			}
		}
	}
	return problems
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
