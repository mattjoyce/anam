package catalogtest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// allowed names test code that may read the embedded catalog, and why.
type allowed struct {
	file   string   // module-relative
	funcs  []string // the top-level functions allowed; nil allows the whole file
	reason string
}

// embeddedAllowList is every test that reads the embedded catalog on purpose. Its
// subject is the catalog the binary carries, so pointed at the fixture copy it would
// test the copy and prove nothing about the binary. R4b (card 255) deletes the
// embedded catalog and empties this list; the rule then stays, with nothing allowed.
// A file holding only allowed tests is listed whole; a file that mixes them with
// tests that moved to the fixture lists its allowed functions.
var embeddedAllowList = []allowed{
	// Slice 1 (#29): the two places the fixture is checked against the binary.
	{file: "internal/catalog/catalogtest/catalogtest_test.go",
		funcs:  []string{"TestFixture_SetVersionMatchesEmbedded", "TestFixture_BytesMatchEmbedded"},
		reason: "the fixture's V and bytes equal the embedded catalog's; deleted with the catalog"},

	// Slice 2 (#31): def and projection.
	{file: "internal/def/bundled_test.go",
		funcs: []string{"TestBundled_HoldsTheHealthDBTypes", "TestBundled_MedTypesShareTheRegimenSeries",
			"TestBundled_LifeEventTypes", "TestBundled_FlightIsDatedWhereItLeft", "TestBundled_DocReferenceIsAReference",
			"TestBundled_LabResultAdmitsAnUntimedDraw", "TestBundled_BloodPressureIsAnOMHBody",
			"TestBundled_NoteAnnotationIsThePersonsWords", "TestBundledVisionPrescription", "TestBundled_WordsNameNoCard"},
		reason: "properties of personal types and their words (WordsNameNoCard reads manifests/ by path); to the deployment repo"},
	{file: "internal/def/heartrateday_test.go",
		reason: "corpus retirement (card 205); to the deployment repo with the corpus tests"},
	{file: "internal/def/readme_test.go",
		reason: "the README table matches the manifests, globbed from manifests/ on disk; to the deployment repo"},
	{file: "internal/def/sharedcore_test.go",
		funcs:  []string{"TestSharedCore_LeavesAgree"},
		reason: "vendor leaves agree with shared-core.json; to the deployment repo"},
	{file: "internal/def/conformance_test.go",
		reason: "IEEE 1752 examples pass the definitions; to the deployment repo"},
	{file: "internal/def/history_test.go",
		reason: "the embedded history carries the renamed types; to the deployment repo"},
	{file: "internal/def/catalog_fs_test.go",
		funcs:  []string{"TestBundledFS_MatchEmbedded", "TestBundledFS_ManifestsHistoryMatch"},
		reason: "a disk copy of fixture.Mattjoyce loads as the embedded definitions do; deleted with the catalog"},
	{file: "internal/projection/catalog_fs_test.go",
		funcs:  []string{"TestBundledFS_MatchEmbedded"},
		reason: "the same cross-check for projections; deleted with the catalog"},
	{file: "internal/projection/projection_test.go",
		funcs:  []string{"TestBundled_ParsesBpAndSleep"},
		reason: "personal projections and the third-segment history; to the deployment repo"},
	{file: "internal/projection/snapshot_test.go",
		funcs:  []string{"TestBundled_MedsIsASnapshot"},
		reason: "the personal meds projection; to the deployment repo"},

	// Slice 3 (#32): catalog.
	{file: "internal/catalog/embedded_test.go",
		reason: "the embedded path of catalog.Load: a disk copy loads the same catalog, built once per process; rewritten for the empty embedded catalog"},

	// Slice 4 (#34): daemon and app.
	{file: "internal/daemon/catalog_drift_test.go",
		reason: "the drift report compares against the embedded catalog (driftConfig clears the directories); rewritten with the new drift baseline"},
	{file: "internal/daemon/catalog_dirs_embedded_test.go",
		reason: "a receipt minted on the embedded catalog verifies after the switch to directory copies (R2b's transition oracle); goes in R4b"},
	{file: "internal/app/heartrateday_retire_internal_test.go",
		reason: "corpus retirement (card 205) over def.Bundled, projection.Bundled and builtInCatalog; to the deployment repo with the corpus tests"},

	// Slice 5 (#33): inspect.
	{file: "internal/inspect/marks_test.go",
		funcs:  []string{"TestTypeStyles_CoverEveryBundledType", "TestDomains_EveryBundledTypeHasAHead"},
		reason: "marks.go covers every type the binary ships (CoverEveryBundledType lists ../def/manifests on disk); R5 (card 256) rewrites them against inspector.yaml"},
}

// pathLabels are strings that spell an embedded catalog path but read nothing from
// disk. The parser cannot tell a label from a path, so they are named here. Each
// entry names one small formatter that builds the label and does nothing else, so a
// read beside it is not excused. They are not reaches, and R4b need not empty this
// list.
var pathLabels = []allowed{
	{file: "internal/def/baseline_test.go", funcs: []string{"goldenLabel"},
		reason: `"manifests/" prefixes the golden file's line labels; the bytes are read from the fixture FS`},
	{file: "internal/projection/baseline_test.go", funcs: []string{"goldenLabel"},
		reason: `"manifests/" prefixes the golden file's line labels; the bytes are read from the fixture FS`},
}

// TestTests_ReadNoEmbeddedCatalog parses every _test.go in the module and fails on
// any read of the embedded catalog outside embeddedAllowList: a use of an embedded
// def or projection symbol, catalog.Load with an empty directory or called through a
// variable, a config catalog directory set to "", a string naming an embedded catalog
// directory on disk, or a call to a test helper in the same package that does any of
// these. It also fails on an entry that no longer reaches the catalog or names a file
// that has gone, so the list cannot outlive what it allows.
//
// The rule stops a test relying on the embedded catalog by accident; a deliberate way
// round it would show in review. What it cannot see: a config that leaves its
// catalog directories unset rather than setting them to "" (a YAML config with no
// definitions_dir is the same case), which the fixture helpers' own tests guard
// (daemon's TestNewConfig_ReadsTheFixtureCatalog, app's TestFixtures_ReadTheFixtureCatalog);
// an empty directory held in a value it cannot resolve in the file (a package-level
// const in another file, a field, a function's result); and a wrapper that reaches
// from outside the test files, in a non-test .go file or through an exported helper
// of another test package. It matches a method call by the method's name, as the
// parser does not know a receiver's type.
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
	for _, p := range checkReaches(s, embeddedAllowList, pathLabels) {
		t.Error(p)
	}
}

// TestAllowList_EntriesAreWellFormed keeps each entry giving a reason and naming a
// file that exists.
func TestAllowList_EntriesAreWellFormed(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	for _, list := range [][]allowed{embeddedAllowList, pathLabels} {
		for _, a := range list {
			if a.reason == "" {
				t.Errorf("%s has no reason", a.file)
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(a.file))); err != nil {
				t.Errorf("%s: %v", a.file, err)
			}
		}
	}
}

// listShape returns what is wrong with the two lists' shape: a file listed twice in
// one list, an empty function set, a function listed twice, and an entry of one list
// that covers a function the other list also covers.
func listShape(allow, labels []allowed) []string {
	var problems []string
	covers := func(a allowed, fn string) bool { return a.funcs == nil || slices.Contains(a.funcs, fn) }
	for _, list := range [][]allowed{allow, labels} {
		for i, a := range list {
			for _, b := range list[:i] {
				if b.file == a.file {
					problems = append(problems, fmt.Sprintf("%s is listed twice in one list", a.file))
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
	}
	for _, a := range allow {
		for _, b := range labels {
			if a.file != b.file {
				continue
			}
			overlap := a.funcs == nil || b.funcs == nil
			for _, f := range a.funcs {
				overlap = overlap || covers(b, f)
			}
			if overlap {
				problems = append(problems, fmt.Sprintf("%s is in both the allow-list and pathLabels for the same function", a.file))
			}
		}
	}
	return problems
}

// checkReaches returns what is wrong: the lists' shape, each reach that no entry
// allows, and each entry (or listed function) that allows nothing or names a file the
// walk did not read. A label excuses only a path. A reach then spreads to every test
// function of the same package that names the function it sits in, transitively, so
// a test calling an allowed helper is held to the list as well.
func checkReaches(s scan, allow, labels []allowed) []string {
	problems := listShape(allow, labels)
	type key struct{ file, fn string }
	usedAllow, usedLabels := map[key]bool{}, map[key]bool{}
	match := func(list []allowed, used map[key]bool, r reach) bool {
		for _, a := range list {
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
	var reaches []reach
	for _, r := range s.reaches {
		if r.Kind == kindPath && match(labels, usedLabels, r) {
			continue
		}
		reaches = append(reaches, r)
	}
	reaches = append(reaches, throughHelpers(reaches, s.refs)...)
	for _, r := range reaches {
		if !match(allow, usedAllow, r) {
			problems = append(problems, fmt.Sprintf("%s reaches the embedded catalog outside the allow-list; read a fixture (catalogtest, fixture.Mattjoyce) instead", r))
		}
	}
	for _, l := range []struct {
		list []allowed
		used map[key]bool
		name string
	}{{allow, usedAllow, "reaches the embedded catalog"}, {labels, usedLabels, "names a catalog path"}} {
		for _, a := range l.list {
			if !slices.Contains(s.files, a.file) {
				problems = append(problems, fmt.Sprintf("listed %s is not a test file in the module; drop its entry", a.file))
				continue
			}
			if a.funcs == nil {
				if !l.used[key{a.file, ""}] {
					problems = append(problems, fmt.Sprintf("listed %s no longer %s; drop its entry", a.file, l.name))
				}
				continue
			}
			for _, f := range a.funcs {
				if !l.used[key{a.file, f}] {
					problems = append(problems, fmt.Sprintf("listed %s %s no longer %s; drop it", a.file, f, l.name))
				}
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
	reaching := map[fn]bool{}
	for _, r := range direct {
		if r.Func != "" {
			reaching[fn{r.Pkg, r.Func}] = true
		}
	}
	names := func(r ref) []string {
		m, ok := strings.CutPrefix(r.To, "*.")
		if !ok {
			if reaching[fn{r.Pkg, r.To}] {
				return []string{r.To}
			}
			return nil
		}
		var out []string
		for f := range reaching {
			if f.pkg == r.Pkg && strings.HasSuffix(f.name, "."+m) {
				out = append(out, f.name)
			}
		}
		slices.Sort(out)
		return out
	}
	var found []reach
	seen := map[[3]string]bool{}
	for changed := true; changed; {
		changed = false
		for _, r := range refs {
			for _, h := range names(r) {
				k := [3]string{r.Pkg, r.From, h}
				if h == r.From || seen[k] {
					continue
				}
				seen[k] = true
				found = append(found, reach{Pkg: r.Pkg, File: r.File, Func: r.From, Line: r.Line, Kind: kindHelper,
					What: "calls " + h + ", which reaches the embedded catalog"})
				if !reaching[fn{r.Pkg, r.From}] {
					reaching[fn{r.Pkg, r.From}] = true
					changed = true
				}
			}
		}
	}
	return found
}
