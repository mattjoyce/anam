package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/projection"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// copyEmbeddedDefs copies the embedded definition catalog to dir, adding
// .DS_Store and notes/extra.json (which must be ignored). Returns an os.DirFS.
func copyEmbeddedDefs(t *testing.T, dir string) {
	t.Helper()
	root := def.EmbeddedFS()

	if err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	}); err != nil {
		t.Fatalf("walk embedded definitions: %v", err)
	}
	// Add ignored entries.
	_ = os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("mock"), 0o644)
	notesDir := filepath.Join(dir, "notes")
	_ = os.MkdirAll(notesDir, 0o755)
	_ = os.WriteFile(filepath.Join(notesDir, "extra.json"), []byte(`{}`), 0o644)
}

// copyEmbeddedProjections copies the embedded projection catalog to dir, adding
// .DS_Store and notes/extra.json.
func copyEmbeddedProjections(t *testing.T, dir string) {
	t.Helper()
	root := projection.EmbeddedFS()

	if err := fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		raw, err := fs.ReadFile(root, path)
		if err != nil {
			return err
		}
		dst := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dst, raw, 0o644)
	}); err != nil {
		t.Fatalf("walk embedded projections: %v", err)
	}
	_ = os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("mock"), 0o644)
	notesDir := filepath.Join(dir, "notes")
	_ = os.MkdirAll(notesDir, 0o755)
	_ = os.WriteFile(filepath.Join(notesDir, "extra.json"), []byte(`{}`), 0o644)
}

// loadEmbedded is a convenience for Load("", "").
func loadEmbedded(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatalf("Load(embedded, embedded): %v", err)
	}
	return cat
}

// ---------------------------------------------------------------------------
// TestLoadEmbeddedAndDiskCopy
// ---------------------------------------------------------------------------
func TestLoadEmbeddedAndDiskCopy(t *testing.T) {
	embedded := loadEmbedded(t)

	defDir := t.TempDir()
	projDir := t.TempDir()
	copyEmbeddedDefs(t, defDir)
	copyEmbeddedProjections(t, projDir)

	mixed, err := catalog.Load(context.Background(), defDir, projDir)
	if err != nil {
		t.Fatalf("Load(disk+disk): %v", err)
	}

	// Compare definition registry contents.
	embeddedAll := embedded.Definitions().All()
	mixedAll := mixed.Definitions().All()
	if len(embeddedAll) != len(mixedAll) {
		t.Fatalf("embedded has %d definitions, disk copy has %d", len(embeddedAll), len(mixedAll))
	}
	for i := range embeddedAll {
		got, want := mixedAll[i], embeddedAll[i]
		if got.Type != want.Type || got.Version != want.Version {
			t.Errorf("definition %d: embedded %s v%d, copy %s v%d",
				i, want.Type, want.Version, got.Type, got.Version)
		}
	}

	// Compare projection registry contents.
	embeddedProj := embedded.Projections().Held()
	mixedProj := mixed.Projections().Held()
	if len(embeddedProj) != len(mixedProj) {
		t.Fatalf("embedded has %d projections, disk copy has %d", len(embeddedProj), len(mixedProj))
	}
	for i := range embeddedProj {
		got, want := mixedProj[i], embeddedProj[i]
		if got.Name != want.Name || got.Version != want.Version || got.Hash != want.Hash {
			t.Errorf("projection %d: embedded %s v%d %s, copy %s v%d %s",
				i, want.Name, want.Version, want.Hash, got.Name, got.Version, got.Hash)
		}
	}

	// Compare set versions.
	if embedded.SetVersion() != mixed.SetVersion() {
		t.Errorf("set version: embedded %s, disk copy %s", embedded.SetVersion(), mixed.SetVersion())
	}

	// Compare manifests (bodies included).
	embeddedMan := embedded.Manifests()
	mixedMan := mixed.Manifests()
	if len(embeddedMan) != len(mixedMan) {
		t.Fatalf("embedded has %d manifests, disk copy has %d", len(embeddedMan), len(mixedMan))
	}
	for i := range embeddedMan {
		got, want := mixedMan[i], embeddedMan[i]
		if got.Type != want.Type || got.Version != want.Version || string(got.Body) != string(want.Body) {
			t.Errorf("manifest %d: embedded %s v%d, copy %s v%d, body matches %v",
				i, want.Type, want.Version, got.Type, got.Version, string(got.Body) == string(want.Body))
		}
	}

	// Compare history.
	embeddedHist := embedded.History()
	mixedHist := mixed.History()
	if len(embeddedHist) != len(mixedHist) {
		t.Fatalf("embedded has %d history entries, disk copy has %d", len(embeddedHist), len(mixedHist))
	}
	for i := range embeddedHist {
		got, want := mixedHist[i], embeddedHist[i]
		if got.Type != want.Type || got.Version != want.Version || string(got.Body) != string(want.Body) {
			t.Errorf("history %d: embedded %s v%d, copy %s v%d",
				i, want.Type, want.Version, got.Type, got.Version)
		}
	}

	// Compare projection bodies, current then history.
	embeddedPM := embedded.ProjectionManifests()
	mixedPM := mixed.ProjectionManifests()
	if len(embeddedPM) != len(mixedPM) {
		t.Fatalf("embedded has %d projection bodies, disk copy has %d", len(embeddedPM), len(mixedPM))
	}
	for i := range embeddedPM {
		got, want := mixedPM[i], embeddedPM[i]
		if got.Name != want.Name || got.Version != want.Version || string(got.Body) != string(want.Body) {
			t.Errorf("projection body %d: embedded %s v%d, copy %s v%d", i, want.Name, want.Version, got.Name, got.Version)
		}
	}

	// Source labels: embedded vs disk.
	if embedded.DefinitionsSource() != "embedded" || embedded.ProjectionsSource() != "embedded" {
		t.Errorf("embedded sources: def=%q proj=%q", embedded.DefinitionsSource(), embedded.ProjectionsSource())
	}
	if mixed.DefinitionsSource() != defDir || mixed.ProjectionsSource() != projDir {
		t.Errorf("disk sources: def=%q (want %q) proj=%q (want %q)",
			mixed.DefinitionsSource(), defDir, mixed.ProjectionsSource(), projDir)
	}

	// Mixed: embedded defs, disk projs.
	mixed2, err := catalog.Load(context.Background(), "", projDir)
	if err != nil {
		t.Fatalf("Load(embedded, disk): %v", err)
	}
	if mixed2.DefinitionsSource() != "embedded" || mixed2.ProjectionsSource() != projDir {
		t.Errorf("mixed sources: def=%q proj=%q", mixed2.DefinitionsSource(), mixed2.ProjectionsSource())
	}
	if mixed2.SetVersion() != embedded.SetVersion() {
		t.Errorf("mixed set version %s != embedded %s", mixed2.SetVersion(), embedded.SetVersion())
	}

	// Mixed: disk defs, embedded projs.
	mixed3, err := catalog.Load(context.Background(), defDir, "")
	if err != nil {
		t.Fatalf("Load(disk, embedded): %v", err)
	}
	if mixed3.DefinitionsSource() != defDir || mixed3.ProjectionsSource() != "embedded" {
		t.Errorf("mixed sources: def=%q proj=%q", mixed3.DefinitionsSource(), mixed3.ProjectionsSource())
	}
	if mixed3.SetVersion() != embedded.SetVersion() {
		t.Errorf("mixed set version %s != embedded %s", mixed3.SetVersion(), embedded.SetVersion())
	}
}

// ---------------------------------------------------------------------------
// TestLoadExternalSetVersion
// ---------------------------------------------------------------------------
func TestLoadExternalSetVersion(t *testing.T) {
	embedded := loadEmbedded(t)

	defDir := t.TempDir()
	projDir := t.TempDir()
	copyEmbeddedDefs(t, defDir)
	copyEmbeddedProjections(t, projDir)

	// Find one definition file and append a space.
	var firstDef string
	entries, err := os.ReadDir(defDir)
	if err != nil {
		t.Fatalf("read def dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			firstDef = e.Name()
			break
		}
	}
	if firstDef == "" {
		t.Fatal("no definition files found")
	}
	raw, err := os.ReadFile(filepath.Join(defDir, firstDef))
	if err != nil {
		t.Fatalf("read %s: %v", firstDef, err)
	}
	raw = append(raw, []byte(" ")...) // trailing space
	if err := os.WriteFile(filepath.Join(defDir, firstDef), raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", firstDef, err)
	}

	// Load the changed catalog.
	cat, err := catalog.Load(context.Background(), defDir, projDir)
	if err != nil {
		t.Fatalf("Load with whitespace change: %v", err)
	}

	// Set version must equal def.SetVersionFS(os.DirFS(defDir)).
	wantSV, err := def.SetVersionFS(os.DirFS(defDir))
	if err != nil {
		t.Fatalf("SetVersionFS(disk): %v", err)
	}
	if cat.SetVersion() != wantSV {
		t.Errorf("SetVersion = %s, want %s (SetVersionFS)", cat.SetVersion(), wantSV)
	}

	// Must differ from the embedded set version.
	if cat.SetVersion() == embedded.SetVersion() {
		t.Error("SetVersion equals embedded after whitespace change")
	}

	// Manifest accessor must preserve the changed bytes.
	manifests := cat.Manifests()
	var found bool
	for _, s := range manifests {
		if string(s.Body) == string(raw) {
			found = true
			break
		}
	}
	if !found {
		t.Error("no manifest preserves the whitespace-changed bytes")
	}

	// A history-only whitespace change must NOT change the set version
	// (history does not contribute to SetVersion).
	histDir := filepath.Join(defDir, "history")
	entries, err = os.ReadDir(histDir)
	if err != nil {
		t.Fatalf("read history dir: %v", err)
	}
	var histName string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			histName = e.Name()
			break
		}
	}
	if histName == "" {
		t.Fatal("embedded catalog has no definition history to edit")
	}
	histRaw, err := os.ReadFile(filepath.Join(histDir, histName))
	if err != nil {
		t.Fatalf("read history %s: %v", histName, err)
	}
	if err := os.WriteFile(filepath.Join(histDir, histName), append(histRaw, ' '), 0o644); err != nil {
		t.Fatalf("write history %s: %v", histName, err)
	}
	reloaded, err := catalog.Load(context.Background(), defDir, projDir)
	if err != nil {
		t.Fatalf("Load after history edit: %v", err)
	}
	if reloaded.SetVersion() != cat.SetVersion() {
		t.Errorf("SetVersion changed after a history-only edit: %s -> %s", cat.SetVersion(), reloaded.SetVersion())
	}
}

// bodiesOf joins manifest bodies so two snapshots of an accessor compare as one value.
func bodiesOf(ms []def.Stored) string {
	var b strings.Builder
	for _, m := range ms {
		fmt.Fprintf(&b, "%s v%d\n%s\n", m.Type, m.Version, m.Body)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// TestLoadFreezesInputs
// ---------------------------------------------------------------------------
func TestLoadFreezesInputs(t *testing.T) {
	defDir := t.TempDir()
	projDir := t.TempDir()
	copyEmbeddedDefs(t, defDir)
	copyEmbeddedProjections(t, projDir)

	first, err := catalog.Load(context.Background(), defDir, projDir)
	if err != nil {
		t.Fatalf("first Load: %v", err)
	}

	firstSV := first.SetVersion()
	firstManifestCount := len(first.Manifests())
	firstBodies := bodiesOf(first.Manifests())

	// Modify and add a definition file.
	var firstDef string
	entries, err := os.ReadDir(defDir)
	if err != nil {
		t.Fatalf("read def dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			firstDef = e.Name()
			break
		}
	}
	if firstDef == "" {
		t.Fatal("no json files in def dir")
	}
	raw, err := os.ReadFile(filepath.Join(defDir, firstDef))
	if err != nil {
		t.Fatalf("read %s: %v", firstDef, err)
	}
	_ = os.WriteFile(filepath.Join(defDir, firstDef), append(raw, []byte(" ")...), 0o644)
	_ = os.WriteFile(filepath.Join(defDir, "test.new.json"),
		[]byte(`{"meta":{"facet_type":"test.new","definition_version":1,"classification":"Normal","validation_mode":"strict","source_schema":"test/x","time_semantics":{"fact_time":{"pointer":"/at","layout":"2006-01-02","zone":"UTC"}},"identity_key":["/at"]},"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","required":["at"],"properties":{"at":{"type":"string","minLength":1}},"additionalProperties":false}}`),
		0o644)

	// The first catalog must be unchanged.
	if first.SetVersion() != firstSV {
		t.Errorf("first catalog's SetVersion changed after source modification: %s -> %s",
			firstSV, first.SetVersion())
	}
	if len(first.Manifests()) != firstManifestCount {
		t.Errorf("first catalog's manifest count changed: was %d, now %d",
			firstManifestCount, len(first.Manifests()))
	}
	if got := bodiesOf(first.Manifests()); got != firstBodies {
		t.Error("first catalog's manifest bodies changed after the source was edited")
	}
	if first.Definitions().Has("test.new") {
		t.Error("first catalog's registry gained a type added to the source after Load")
	}

	// A second load observes the changes.
	second, err := catalog.Load(context.Background(), defDir, projDir)
	if err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if second.SetVersion() == firstSV {
		t.Error("second Load's SetVersion is unchanged — did not observe the source modification")
	}
}

// ---------------------------------------------------------------------------
// TestLoadCopiesBodies
// ---------------------------------------------------------------------------
func TestLoadCopiesBodies(t *testing.T) {
	cat := loadEmbedded(t)

	sv := cat.SetVersion()
	wantManifests := bodiesOf(cat.Manifests())
	wantHistory := bodiesOf(cat.History())
	wantProj := projBodiesOf(cat.ProjectionManifests())

	// Overwrite every returned byte in place, then drop entries from the slices.
	for _, ms := range [][]def.Stored{cat.Manifests(), cat.History()} {
		if len(ms) == 0 {
			t.Fatal("embedded catalog has no definition bodies to mutate")
		}
		for i := range ms {
			for j := range ms[i].Body {
				ms[i].Body[j] = 'X'
			}
		}
	}
	pms := cat.ProjectionManifests()
	if len(pms) == 0 {
		t.Fatal("embedded catalog has no projection bodies to mutate")
	}
	for i := range pms {
		for j := range pms[i].Body {
			pms[i].Body[j] = 'X'
		}
	}

	if got := bodiesOf(cat.Manifests()); got != wantManifests {
		t.Error("Manifests() bodies changed after a caller overwrote returned bytes")
	}
	if got := bodiesOf(cat.History()); got != wantHistory {
		t.Error("History() bodies changed after a caller overwrote returned bytes")
	}
	if got := projBodiesOf(cat.ProjectionManifests()); got != wantProj {
		t.Error("ProjectionManifests() bodies changed after a caller overwrote returned bytes")
	}
	if cat.SetVersion() != sv {
		t.Errorf("SetVersion changed after mutation: %s -> %s", sv, cat.SetVersion())
	}
}

// projBodiesOf joins projection bodies so two snapshots compare as one value.
func projBodiesOf(ps []projection.Stored) string {
	var b strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&b, "%s v%d\n%s\n", p.Name, p.Version, p.Body)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// TestLoadRejectsInvalidCatalog
// ---------------------------------------------------------------------------
func TestLoadRejectsInvalidCatalog(t *testing.T) {
	ctx := context.Background()

	validDefDir := t.TempDir()
	validProjDir := t.TempDir()
	copyEmbeddedDefs(t, validDefDir)
	copyEmbeddedProjections(t, validProjDir)

	missing := filepath.Join(t.TempDir(), "absent")
	fileRoot := filepath.Join(t.TempDir(), "a-file")
	if err := os.WriteFile(fileRoot, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write file root: %v", err)
	}
	defFileHistory := makeFileHistory(t, validDefDir)
	projFileHistory := makeFileHistory(t, validProjDir)

	type caseT struct {
		name         string
		defDir       string
		projDir      string
		wantSentinel error
		wantField    string // definitions_dir or projections_dir
		wantInMsg    string // a path or file name the message must name
	}

	badDef, badDefHist := makeBadDefDir(t, validDefDir), makeBadDefHistory(t, validDefDir)
	badProj, badProjHist := makeBadProjDir(t, validProjDir), makeBadProjHistory(t, validProjDir)
	defMismatch, projMismatch := makeNameMismatchDef(t, validDefDir), makeNameMismatchProj(t, validProjDir)
	cases := []caseT{
		{"missing def dir", missing, validProjDir, fs.ErrNotExist, "definitions_dir", missing},
		{"missing proj dir", validDefDir, missing, fs.ErrNotExist, "projections_dir", missing},
		{"def root is a file", fileRoot, validProjDir, fs.ErrInvalid, "definitions_dir", fileRoot},
		{"proj root is a file", validDefDir, fileRoot, fs.ErrInvalid, "projections_dir", fileRoot},
		{"def history is a file", defFileHistory, validProjDir, fs.ErrInvalid, "definitions_dir", "history"},
		{"proj history is a file", validDefDir, projFileHistory, fs.ErrInvalid, "projections_dir", "history"},
		{"bad def manifest", badDef, validProjDir, def.ErrBadManifest, "definitions_dir", badDef},
		{"bad def history", badDefHist, validProjDir, def.ErrBadManifest, "definitions_dir", "bad.v1.json"},
		{"bad proj manifest", validDefDir, badProj, projection.ErrBadTemplate, "projections_dir", badProj},
		{"bad proj history", validDefDir, badProjHist, projection.ErrBadTemplate, "projections_dir", "bad.v1.json"},
		{"def name mismatch", defMismatch, validProjDir, def.ErrBadManifest, "definitions_dir", "wrong-name.json"},
		{"proj name mismatch", validDefDir, projMismatch, projection.ErrBadTemplate, "projections_dir", "wrong-name.json"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			cat, err := catalog.Load(ctx, c.defDir, c.projDir)
			if err == nil {
				t.Fatalf("Load succeeded, expected error wrapping %v", c.wantSentinel)
			}
			if cat != nil {
				t.Error("Load returned a catalog alongside its error")
			}
			if !errors.Is(err, catalog.ErrInvalid) {
				t.Errorf("err = %v, want catalog.ErrInvalid", err)
			}
			if !errors.Is(err, c.wantSentinel) {
				t.Errorf("err = %v, want %v", err, c.wantSentinel)
			}
			for _, want := range []string{c.wantField, c.wantInMsg} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("err = %v, want it to name %q", err, want)
				}
			}
		})
	}
}

// makeFileHistory copies a valid catalog and replaces its history/ directory with a
// regular file of that name.
func makeFileHistory(t *testing.T, srcDir string) string {
	t.Helper()
	dir := t.TempDir()
	copyAll(t, srcDir, dir)
	if err := os.RemoveAll(filepath.Join(dir, "history")); err != nil {
		t.Fatalf("remove history: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "history"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write history file: %v", err)
	}
	return dir
}

// --- helpers for invalid test cases ---

func makeBadDefDir(t *testing.T, srcDir string) string {
	t.Helper()
	dir := t.TempDir()
	copyAll(t, srcDir, dir)
	corruptOneJSON(t, dir)
	return dir
}

func makeBadDefHistory(t *testing.T, srcDir string) string {
	t.Helper()
	dir := t.TempDir()
	copyAll(t, srcDir, dir)
	// Add a malformed history file.
	histDir := filepath.Join(dir, "history")
	_ = os.MkdirAll(histDir, 0o755)
	_ = os.WriteFile(filepath.Join(histDir, "bad.v1.json"), []byte(`{not json}`), 0o644)
	return dir
}

func makeBadProjDir(t *testing.T, srcDir string) string {
	t.Helper()
	dir := t.TempDir()
	copyAll(t, srcDir, dir)
	corruptOneJSON(t, dir)
	return dir
}

func makeBadProjHistory(t *testing.T, srcDir string) string {
	t.Helper()
	dir := t.TempDir()
	copyAll(t, srcDir, dir)
	histDir := filepath.Join(dir, "history")
	_ = os.MkdirAll(histDir, 0o755)
	_ = os.WriteFile(filepath.Join(histDir, "bad.v1.json"),
		[]byte(`{"name": "nonexistent", "version": 99, "types":[],"statuses":[],"max_classification":"Normal","from":"2024-01-01","to":"2024-12-31"}`), 0o644)
	return dir
}

func makeNameMismatchDef(t *testing.T, srcDir string) string {
	t.Helper()
	dir := t.TempDir()
	copyAll(t, srcDir, dir)
	renameOneJSON(t, dir, "wrong-name.json")
	return dir
}

func makeNameMismatchProj(t *testing.T, srcDir string) string {
	t.Helper()
	dir := t.TempDir()
	copyAll(t, srcDir, dir)
	renameOneJSON(t, dir, "wrong-name.json")
	return dir
}

func copyAll(t *testing.T, src, dst string) {
	t.Helper()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatalf("read %s: %v", src, err)
	}
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			if err := os.MkdirAll(dstPath, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", dstPath, err)
			}
			copyAll(t, srcPath, dstPath)
			continue
		}
		raw, err := os.ReadFile(srcPath)
		if err != nil {
			t.Fatalf("read %s: %v", srcPath, err)
		}
		if err := os.WriteFile(dstPath, raw, 0o644); err != nil {
			t.Fatalf("write %s: %v", dstPath, err)
		}
	}
}

func corruptOneJSON(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			path := filepath.Join(dir, e.Name())
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			_ = os.WriteFile(path, append([]byte(`corrupt-`), raw...), 0o644)
			return
		}
	}
}

func renameOneJSON(t *testing.T, dir, newName string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".json" {
			src := filepath.Join(dir, e.Name())
			raw, err := os.ReadFile(src)
			if err != nil {
				t.Fatalf("read %s: %v", e.Name(), err)
			}
			_ = os.WriteFile(filepath.Join(dir, newName), raw, 0o644)
			_ = os.Remove(src)
			return
		}
	}
}

// ---------------------------------------------------------------------------
// TestLoadRejectsConflictingDefinitions
// ---------------------------------------------------------------------------
func TestLoadRejectsConflictingDefinitions(t *testing.T) {
	ctx := context.Background()

	validProjDir := t.TempDir()
	copyEmbeddedProjections(t, validProjDir)

	// Find one definition from the embedded catalog.
	embeddedDefs, err := def.Manifests()
	if err != nil {
		t.Fatalf("Manifests: %v", err)
	}
	if len(embeddedDefs) == 0 {
		t.Fatal("no embedded definitions")
	}
	first := embeddedDefs[0]

	// Create a dir with only first's type, then add a conflicting history entry
	// at the same type and version with a different body.
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, string(first.Type)+".json"), first.Body, 0o644)
	histDir := filepath.Join(dir, "history")
	_ = os.MkdirAll(histDir, 0o755)

	differentBody := []byte(fmt.Sprintf(`{"meta":{"facet_type":"%s","definition_version":%d,"classification":"Normal","validation_mode":"strict","source_schema":"other","time_semantics":{"fact_time":{"pointer":"/at","layout":"2006-01-02","zone":"UTC"}},"identity_key":["/at"]},"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","properties":{"x":{"type":"string"}},"additionalProperties":false}}`,
		first.Type, first.Version))
	_ = os.WriteFile(filepath.Join(histDir, fmt.Sprintf("%s.v%d.json", first.Type, first.Version)), differentBody, 0o644)

	_, err = catalog.Load(ctx, dir, validProjDir)
	if err == nil {
		t.Fatal("Load with conflicting definition succeeded, expected error")
	}
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("err = %v, want catalog.ErrInvalid", err)
	}
	if !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("err = %v, want def.ErrBadManifest", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"definitions_dir",
		fmt.Sprintf("%s v%d", first.Type, first.Version),
		string(first.Type) + ".json",
		fmt.Sprintf("history/%s.v%d.json", first.Type, first.Version),
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message %q does not name %q", msg, want)
		}
	}

	// An identical canonically equal pair must succeed.
	dir2 := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir2, string(first.Type)+".json"), first.Body, 0o644)
	histDir2 := filepath.Join(dir2, "history")
	_ = os.MkdirAll(histDir2, 0o755)
	_ = os.WriteFile(filepath.Join(histDir2, fmt.Sprintf("%s.v%d.json", first.Type, first.Version)), first.Body, 0o644)

	cat2, err := catalog.Load(ctx, dir2, validProjDir)
	if err != nil {
		t.Fatalf("Load with identical canonically equal pair: %v", err)
	}
	// Verify the catalog includes the type.
	manifests := cat2.Manifests()
	var count int
	for _, s := range manifests {
		if s.Type == first.Type && s.Version == first.Version {
			count++
		}
	}
	if count < 1 {
		t.Errorf("identical pair catalog has no entries for %s v%d", first.Type, first.Version)
	}
}

// ---------------------------------------------------------------------------
// TestLoadRejectsProjectionHistoryConflict
// ---------------------------------------------------------------------------
func TestLoadRejectsProjectionHistoryConflict(t *testing.T) {
	ctx := context.Background()
	validDefDir := t.TempDir()
	copyEmbeddedDefs(t, validDefDir)

	// Get one embedded projection template.
	embeddedProj, err := projection.Bundled()
	if err != nil {
		t.Fatalf("projection.Bundled: %v", err)
	}
	templates := embeddedProj.Held()
	if len(templates) == 0 {
		t.Fatal("no embedded projections")
	}
	first := templates[0]

	// Create a projection dir with one template and a history file at the same
	// version, which projection.BundledFS rejects.
	projDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(projDir, first.Name+".json"), first.Body, 0o644)
	histDir := filepath.Join(projDir, "history")
	_ = os.MkdirAll(histDir, 0o755)
	_ = os.WriteFile(filepath.Join(histDir, fmt.Sprintf("%s.v%d.json", first.Name, first.Version)),
		[]byte(`{"name":"`+first.Name+`","version":`+strconv.Itoa(first.Version)+`,"types":[],"statuses":[],"max_classification":"Normal","from":"2024-01-01T00:00:00.000000000Z","to":"2024-12-31T00:00:00.000000000Z"}`),
		0o644)

	_, err = catalog.Load(ctx, validDefDir, projDir)
	if err == nil {
		t.Fatal("Load with conflicting projection version succeeded, expected error")
	}
	if !errors.Is(err, catalog.ErrInvalid) {
		t.Errorf("err = %v, want catalog.ErrInvalid", err)
	}
	if !errors.Is(err, projection.ErrBadTemplate) {
		t.Errorf("err = %v, want projection.ErrBadTemplate", err)
	}
	msg := err.Error()
	if !strings.Contains(msg, first.Name) {
		t.Errorf("error message %q does not name the template %s", msg, first.Name)
	}
}

// ---------------------------------------------------------------------------
// TestLoadReturnsNoPartialCatalog
// ---------------------------------------------------------------------------
func TestLoadReturnsNoPartialCatalog(t *testing.T) {
	ctx := context.Background()
	validDefDir := t.TempDir()
	validProjDir := t.TempDir()
	copyEmbeddedDefs(t, validDefDir)
	copyEmbeddedProjections(t, validProjDir)

	// Valid defs + invalid projs.
	badProjDir := makeBadProjDir(t, validProjDir)
	cat, err := catalog.Load(ctx, validDefDir, badProjDir)
	if err == nil {
		t.Fatal("Load(valid def, bad proj) succeeded")
	}
	if cat != nil {
		t.Error("Load returned a non-nil catalog after error")
	}
	if !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, projection.ErrBadTemplate) {
		t.Errorf("err = %v, want catalog.ErrInvalid and projection.ErrBadTemplate", err)
	}
	if !strings.Contains(err.Error(), "projections_dir") {
		t.Errorf("err = %v, want it to name projections_dir", err)
	}

	// Invalid defs + valid projs.
	badDefDir := makeBadDefDir(t, validDefDir)
	cat2, err := catalog.Load(ctx, badDefDir, validProjDir)
	if err == nil {
		t.Fatal("Load(bad def, valid proj) succeeded")
	}
	if cat2 != nil {
		t.Error("Load returned a non-nil catalog after error")
	}
	if !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, def.ErrBadManifest) {
		t.Errorf("err = %v, want catalog.ErrInvalid and def.ErrBadManifest", err)
	}
	if !strings.Contains(err.Error(), "definitions_dir") {
		t.Errorf("err = %v, want it to name definitions_dir", err)
	}
}

// ---------------------------------------------------------------------------
// TestLoadEmptyDirectories
// ---------------------------------------------------------------------------
func TestLoadEmptyDirectories(t *testing.T) {
	ctx := context.Background()
	defDir := t.TempDir()
	projDir := t.TempDir()

	cat, err := catalog.Load(ctx, defDir, projDir)
	if err != nil {
		t.Fatalf("Load with empty dirs: %v", err)
	}

	// Empty registries.
	if len(cat.Definitions().All()) != 0 {
		t.Errorf("empty defs: All() has entries")
	}
	if len(cat.Projections().Held()) != 0 {
		t.Errorf("empty projs: Held() has entries")
	}

	// Empty manifest accessors.
	if len(cat.Manifests()) != 0 {
		t.Errorf("empty dirs: Manifests() has entries")
	}
	if len(cat.History()) != 0 {
		t.Errorf("empty dirs: History() has entries")
	}
	if len(cat.ProjectionManifests()) != 0 {
		t.Errorf("empty dirs: ProjectionManifests() has entries")
	}

	// Set version must match SetVersionFS over an empty root.
	wantSV, err := def.SetVersionFS(os.DirFS(defDir))
	if err != nil {
		t.Fatalf("SetVersionFS(empty): %v", err)
	}
	if cat.SetVersion() != wantSV {
		t.Errorf("SetVersion = %s, want %s (empty-root SetVersionFS)", cat.SetVersion(), wantSV)
	}

	// Source labels should be the directory paths, not "embedded".
	if cat.DefinitionsSource() != defDir || cat.ProjectionsSource() != projDir {
		t.Errorf("sources: def=%q proj=%q, want dirs", cat.DefinitionsSource(), cat.ProjectionsSource())
	}
}

// ---------------------------------------------------------------------------
// TestLoadCanceled
// ---------------------------------------------------------------------------
func TestLoadCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cat, err := catalog.Load(ctx, "", "")
	if err == nil {
		t.Fatal("Load with canceled context succeeded")
	}
	if cat != nil {
		t.Error("Load returned a non-nil catalog after cancellation")
	}
	if !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want catalog.ErrInvalid and context.Canceled", err)
	}
}

// cancelAt is a context that reports itself canceled from its nth Err call on, so a
// test can cancel a load at every point Load checks.
type cancelAt struct {
	context.Context
	n, calls int
}

func (c *cancelAt) Err() error {
	c.calls++
	if c.n > 0 && c.calls >= c.n {
		return context.Canceled
	}
	return nil
}

// TestLoadCanceledMidway cancels at each check Load makes, from the first to the one
// before it returns: every point fails with both sentinels and no catalog. Load
// checks before every file it reads, so it makes more checks than there are files.
// It loads a disk copy: the embedded catalog is built once per process, so a load of
// it reads no files after the first.
func TestLoadCanceledMidway(t *testing.T) {
	defDir, projDir := t.TempDir(), t.TempDir()
	copyEmbeddedDefs(t, defDir)
	copyEmbeddedProjections(t, projDir)
	counter := &cancelAt{Context: context.Background()}
	if _, err := catalog.Load(counter, defDir, projDir); err != nil {
		t.Fatalf("Load with a context that never cancels: %v", err)
	}
	total := counter.calls
	files := 0
	for _, root := range []fs.FS{def.EmbeddedFS(), projection.EmbeddedFS()} {
		_ = fs.WalkDir(root, ".", func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files++
			}
			return nil
		})
	}
	if total <= files {
		t.Fatalf("Load checked its context %d times for %d files, want a check per file", total, files)
	}
	for n := 1; n <= total; n++ {
		cat, err := catalog.Load(&cancelAt{Context: context.Background(), n: n}, defDir, projDir)
		if cat != nil {
			t.Fatalf("cancel at check %d of %d: Load returned a catalog", n, total)
		}
		if !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancel at check %d of %d: err = %v, want catalog.ErrInvalid and context.Canceled", n, total, err)
		}
	}
}

// TestLoadEmbeddedIsBuiltOnce: the embedded catalog is shared, and a canceled
// context still refuses it, even once it is built.
func TestLoadEmbeddedIsBuiltOnce(t *testing.T) {
	a, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Error("two loads of the embedded catalog built two catalogs, want one shared")
	}
	for n := 1; n <= 2; n++ {
		cat, err := catalog.Load(&cancelAt{Context: context.Background(), n: n}, "", "")
		if cat != nil || !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, context.Canceled) {
			t.Errorf("cancel at check %d: cat %v, err %v, want nil and both sentinels", n, cat, err)
		}
	}
}

// CheckRegistered agrees with a registered body equal to the catalog's once
// canonicalized, ignores versions the catalog does not hold, and refuses a different
// body at a version it holds, naming the type and version.
func TestCheckRegistered(t *testing.T) {
	cat, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	m := cat.Manifests()[0]
	reformatted := append([]byte("\n  "), m.Body...)
	other := def.Stored{Type: m.Type, Version: m.Version + 1000, Body: []byte(`{"anything":"else"}`)}
	if err := cat.CheckRegistered([]def.Stored{{Type: m.Type, Version: m.Version, Body: reformatted}, other}); err != nil {
		t.Errorf("CheckRegistered of a reformatted body and an unheld version = %v, want nil", err)
	}

	var body map[string]any
	if err := json.Unmarshal(m.Body, &body); err != nil {
		t.Fatal(err)
	}
	body["x_contradiction"] = true
	changed, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	err = cat.CheckRegistered([]def.Stored{{Type: m.Type, Version: m.Version, Body: changed}})
	if !errors.Is(err, def.ErrBadManifest) {
		t.Fatalf("CheckRegistered of a contradicting body = %v, want def.ErrBadManifest", err)
	}
	if want := fmt.Sprintf("%s v%d", m.Type, m.Version); !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name %s", err, want)
	}
}
