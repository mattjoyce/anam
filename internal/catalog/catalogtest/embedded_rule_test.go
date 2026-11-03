package catalogtest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
// disk. The parser cannot tell a label from a path, so they are named here. They are
// not reaches, and R4b need not empty this list.
var pathLabels = []allowed{
	{file: "internal/def/baseline_test.go", funcs: []string{"baselineLines"},
		reason: `"manifests/" prefixes the golden file's line labels for files read from the fixture FS`},
	{file: "internal/projection/baseline_test.go", funcs: []string{"baselineLines"},
		reason: `"manifests/" prefixes the golden file's line labels for files read from the fixture FS`},
}

// TestTests_ReadNoEmbeddedCatalog parses every _test.go in the module and fails on
// any read of the embedded catalog outside embeddedAllowList: a use of an embedded
// def or projection symbol, catalog.Load with an empty directory, a config catalog
// directory set to "", or a string naming an embedded catalog directory on disk. It
// also fails on an entry that no longer reaches the catalog or names a file that has
// gone, so the list cannot outlive what it allows.
//
// It cannot see a config that leaves its directories unset rather than setting them
// to "", or a YAML config that names none; catalogtest.Config is how a test's daemon
// gets a fixture.
func TestTests_ReadNoEmbeddedCatalog(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	reaches, files, err := findReaches(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 100 {
		t.Fatalf("read %d test files under %s, want the module's", len(files), root)
	}
	for _, p := range checkReaches(reaches, files, embeddedAllowList, pathLabels) {
		t.Error(p)
	}
}

// TestAllowList_EntriesAreWellFormed keeps each entry giving a reason, naming a file
// once, and each listed function once.
func TestAllowList_EntriesAreWellFormed(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	for _, list := range [][]allowed{embeddedAllowList, pathLabels} {
		seen := map[string]bool{}
		for _, a := range list {
			if a.reason == "" {
				t.Errorf("%s has no reason", a.file)
			}
			if seen[a.file] {
				t.Errorf("%s is listed twice", a.file)
			}
			seen[a.file] = true
			if a.funcs != nil && len(a.funcs) == 0 {
				t.Errorf("%s lists an empty set of functions; use nil for the whole file", a.file)
			}
			for i, f := range a.funcs {
				if slices.Contains(a.funcs[:i], f) {
					t.Errorf("%s lists %s twice", a.file, f)
				}
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(a.file))); err != nil {
				t.Errorf("%s: %v", a.file, err)
			}
		}
	}
}

// checkReaches returns what is wrong: each reach that no entry allows, and each entry
// (or listed function) that allows nothing or names a file the walk did not read.
func checkReaches(reaches []reach, files []string, allow, labels []allowed) []string {
	type key struct{ file, fn string }
	used := map[key]bool{}
	match := func(list []allowed, r reach) bool {
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
	var problems []string
	for _, r := range reaches {
		if r.Kind == kindPath && match(labels, r) {
			continue
		}
		if !match(allow, r) {
			problems = append(problems, fmt.Sprintf("%s reaches the embedded catalog outside the allow-list; read a fixture (catalogtest, fixture.Mattjoyce) instead", r))
		}
	}
	for _, l := range []struct {
		list []allowed
		name string
	}{{allow, "reaches the embedded catalog"}, {labels, "names a catalog path"}} {
		for _, a := range l.list {
			if !slices.Contains(files, a.file) {
				problems = append(problems, fmt.Sprintf("listed %s is not a test file in the module; drop its entry", a.file))
				continue
			}
			if a.funcs == nil {
				if !used[key{a.file, ""}] {
					problems = append(problems, fmt.Sprintf("listed %s no longer %s; drop its entry", a.file, l.name))
				}
				continue
			}
			for _, f := range a.funcs {
				if !used[key{a.file, f}] {
					problems = append(problems, fmt.Sprintf("listed %s %s no longer %s; drop it", a.file, f, l.name))
				}
			}
		}
	}
	return problems
}
