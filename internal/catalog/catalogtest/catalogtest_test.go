package catalogtest_test

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// v is the definition set version the binary reports at de2981c, the R0 oracle the
// mattjoyce fixture must reproduce.
const v = "sha256:b8cc55e0a2f6d14de76a15a892eaa5cd53577ae857cefff09730bbbcb6d9e969"

// TestFixture_SetVersionIsV pins the fixture's definition set version to the literal
// V, over the files and over the catalog Load builds from a directory copy.
func TestFixture_SetVersionIsV(t *testing.T) {
	t.Parallel()
	defs, _ := fixture.Mattjoyce()
	got, err := def.SetVersionFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	if got != v {
		t.Errorf("SetVersionFS(fixture.Mattjoyce) = %s, want %s", got, v)
	}
	if got := catalogtest.Catalog(t).SetVersion(); got != v {
		t.Errorf("catalogtest.Catalog SetVersion = %s, want %s", got, v)
	}
}

// TestHelpers_BuildTheFixture checks each helper is built over fixture.Mattjoyce.
func TestHelpers_BuildTheFixture(t *testing.T) {
	t.Parallel()
	if got := len(catalogtest.Definitions(t).All()); got != 44 {
		t.Errorf("Definitions: %d types, want 44", got)
	}
	if got := len(catalogtest.Projections(t).Names()); got != 12 {
		t.Errorf("Projections: %d templates, want 12", got)
	}
	c := catalogtest.Catalog(t)
	if got := len(c.Definitions().All()); got != 44 {
		t.Errorf("Catalog: %d types, want 44", got)
	}
	if c.DefinitionsSource() == "embedded" || c.ProjectionsSource() == "embedded" {
		t.Errorf("Catalog sources = %q, %q; want directories", c.DefinitionsSource(), c.ProjectionsSource())
	}
	if catalogtest.Catalog(t) != c {
		t.Error("Catalog built twice; want once per process")
	}

	var cfg config.Config
	catalogtest.Config(t, &cfg)
	if cfg.DefinitionsDir == "" || cfg.ProjectionsDir == "" {
		t.Fatalf("Config left dirs %q, %q", cfg.DefinitionsDir, cfg.ProjectionsDir)
	}
	loaded, err := catalog.Load(context.Background(), cfg.DefinitionsDir, cfg.ProjectionsDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := loaded.SetVersion(); got != v {
		t.Errorf("catalog over Config's dirs: SetVersion = %s, want %s", got, v)
	}
}

// TestMinimal_Loads checks the minimal catalog is a valid catalog on its own: Load
// accepts it from directories, its version is computable and is not V, and its
// Definitions and templates are the ones it says it holds.
func TestMinimal_Loads(t *testing.T) {
	t.Parallel()
	defs, projs := fixture.Minimal()
	defsDir, projsDir := fixture.Dirs(t, defs, projs)
	c, err := catalog.Load(context.Background(), defsDir, projsDir)
	if err != nil {
		t.Fatal(err)
	}
	var types []string
	for _, d := range c.Definitions().All() {
		types = append(types, string(d.Type))
	}
	sort.Strings(types)
	if got, want := types, []string{"record.document", "record.edge", "test.measure"}; !equal(got, want) {
		t.Errorf("types = %v, want %v", got, want)
	}
	if got, want := c.Projections().Names(), []string{"measures"}; !equal(got, want) {
		t.Errorf("projections = %v, want %v", got, want)
	}
	tmpl, err := c.Projections().Lookup("measures")
	if err != nil {
		t.Fatal(err)
	}
	if got := tmpl.Covers(c.Definitions().All()); len(got) != 1 || got[0] != "test.measure" {
		t.Errorf("measures covers %v, want [test.measure]", got)
	}
	sv, err := def.SetVersionFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	if sv != c.SetVersion() || sv == v || len(sv) != len(v) {
		t.Errorf("minimal SetVersion = %s (catalog %s), want a sha256 that is not V", sv, c.SetVersion())
	}
}

// TestMinimal_RecordTypesInUTC checks the minimal catalog's record.* types read their
// fact time in UTC, pass the Guard on an empty store and over their own manifests,
// and admit a payload: a non-Sydney record.* for R4b to start from.
func TestMinimal_RecordTypesInUTC(t *testing.T) {
	t.Parallel()
	defs, _ := fixture.Minimal()
	reg, err := def.BundledFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Guard(nil, reg); err != nil {
		t.Errorf("Guard on an empty store: %v", err)
	}
	stored, err := def.ManifestsFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Guard(stored, reg); err != nil {
		t.Errorf("Guard over the catalog's own manifests: %v", err)
	}
	for _, c := range []struct {
		typ     string
		payload string
	}{
		{"record.edge", `{"id":"note:1","at":"2026-11-03 09:30","text":"words",` +
			`"source_time":{"value":"2026-11-03 09:30","zone":"UTC","offset":"+00:00","basis":"configuration"}}`},
		{"record.document", `{"path":"a.pdf","sha256":"` + string(bytes.Repeat([]byte("0"), 64)) + `",` +
			`"media_type":"application/pdf","bytes":1,"date":"2026-11-03",` +
			`"source_time":{"value":"2026-11-03","zone":"UTC","basis":"configuration"}}`},
	} {
		d, ok := reg.Lookup(facet.Type(c.typ))
		if !ok {
			t.Errorf("%s: not in the minimal catalog", c.typ)
			continue
		}
		if d.FactTime.Zone != "UTC" {
			t.Errorf("%s: fact time zone %q, want UTC", c.typ, d.FactTime.Zone)
		}
		doc, err := def.Decode([]byte(c.payload))
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Check(doc); err != nil {
			t.Errorf("%s: a UTC payload is refused: %v", c.typ, err)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
