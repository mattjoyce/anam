package catalogtest_test

import (
	"bytes"
	"context"
	"sort"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// v is the definition set version the deployment's catalog reports, the live V since
// R0. No engine fixture may reproduce it: the overlay in anam_mattjoyce pins it over
// that catalog, and the engine holds no copy (card 255, R4b).
const v = "sha256:b8cc55e0a2f6d14de76a15a892eaa5cd53577ae857cefff09730bbbcb6d9e969"

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

// recorder is a testing.TB that records a Fatalf instead of stopping, so a test can
// see a Require helper fail. The helpers return after Fatalf, so nothing runs on.
type recorder struct {
	testing.TB
	failed bool
}

func (r *recorder) Helper()               {}
func (r *recorder) Fatalf(string, ...any) { r.failed = true }

// TestRequire_PassesOnlyTheEmptyCatalog checks the helpers a test on emptyOnPurpose
// checks its selection with: each passes the empty component it names and fails a
// catalog with that component from a directory, a fixture, or no catalog at all.
func TestRequire_PassesOnlyTheEmptyCatalog(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	empty, err := catalog.Load(ctx, "", "")
	if err != nil {
		t.Fatal(err)
	}
	catalogtest.RequireEmpty(t, empty)

	defs, projs := fixture.Minimal()
	defsDir, _ := fixture.Dirs(t, defs, projs)
	noProjs, err := catalog.Load(ctx, defsDir, "")
	if err != nil {
		t.Fatal(err)
	}
	catalogtest.RequireNoProjections(t, noProjs)
	emptyDirs, err := catalog.Load(ctx, t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	refused, err := catalog.Load(&canceled{ctx}, "", "")
	catalogtest.RequireRefused(t, refused, err)

	for _, c := range []struct {
		name    string
		require func(testing.TB)
	}{
		{"Empty, the engine fixture", func(tb testing.TB) { catalogtest.RequireEmpty(tb, catalogtest.Engine.Catalog(t)) }},
		{"Empty, definitions from a directory", func(tb testing.TB) { catalogtest.RequireEmpty(tb, noProjs) }},
		{"Empty, empty directories", func(tb testing.TB) { catalogtest.RequireEmpty(tb, emptyDirs) }},
		{"Empty, no catalog", func(tb testing.TB) { catalogtest.RequireEmpty(tb, nil) }},
		{"NoDefinitions, definitions from a directory", func(tb testing.TB) { catalogtest.RequireNoDefinitions(tb, noProjs) }},
		{"NoProjections, the engine fixture", func(tb testing.TB) { catalogtest.RequireNoProjections(tb, catalogtest.Engine.Catalog(t)) }},
		{"NoProjections, no catalog", func(tb testing.TB) { catalogtest.RequireNoProjections(tb, nil) }},
		{"Refused, a catalog", func(tb testing.TB) { catalogtest.RequireRefused(tb, empty, nil) }},
		{"Refused, another error", func(tb testing.TB) { catalogtest.RequireRefused(tb, nil, context.Canceled) }},
	} {
		r := &recorder{TB: t}
		c.require(r)
		if !r.failed {
			t.Errorf("Require%s passed", c.name)
		}
	}
}

// canceled is a context that is already canceled.
type canceled struct{ context.Context }

func (canceled) Err() error { return context.Canceled }
