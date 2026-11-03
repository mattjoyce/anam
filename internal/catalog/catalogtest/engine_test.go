package catalogtest_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/projection"
)

// engineV is fixture.Engine's definition set version. A byte changed in a current
// definition moves it; history and projections do not count toward it.
const engineV = "sha256:e48a8f30adae725880b2cb8859f014e7538150eec08cbe5d279a8496420552f7"

// engineTypes and engineTemplates are what fixture.Engine holds in force.
var (
	engineTypes = []string{"record.document", "record.edge", "test.course.changed", "test.course.started",
		"test.course.stopped", "test.labelled", "test.measure", "test.pair", "test.reading", "test.templated"}
	engineTemplates = []string{"course", "everything", "measures", "normal"}
)

// TestEngine_SetVersionIsPinned pins the engine catalog's version to a literal, over
// the files and over the catalog Load builds from a directory copy, and checks it is
// not the personal V.
func TestEngine_SetVersionIsPinned(t *testing.T) {
	t.Parallel()
	defs, _ := fixture.Engine()
	got, err := def.SetVersionFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	if got != engineV {
		t.Errorf("SetVersionFS(fixture.Engine) = %s, want %s", got, engineV)
	}
	if got := catalogtest.Engine.Catalog(t).SetVersion(); got != engineV {
		t.Errorf("catalogtest.Engine.Catalog SetVersion = %s, want %s", got, engineV)
	}
	if engineV == v {
		t.Error("the engine catalog's version is the personal V")
	}
}

// TestEngine_LoadsFromADirectoryCopy checks Load accepts the engine catalog from
// directories with its history, and that each Engine helper is built over it, once.
func TestEngine_LoadsFromADirectoryCopy(t *testing.T) {
	t.Parallel()
	defs, projs := fixture.Engine()
	defsDir, projsDir := fixture.Dirs(t, defs, projs)
	c, err := catalog.Load(context.Background(), defsDir, projsDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := typesOf(c.Definitions()); !slices.Equal(got, engineTypes) {
		t.Errorf("types = %v, want %v", got, engineTypes)
	}
	if got := c.Projections().Names(); !slices.Equal(got, engineTemplates) {
		t.Errorf("projections = %v, want %v", got, engineTemplates)
	}
	if got := len(c.History()); got != 6 {
		t.Errorf("definition history: %d versions, want 6", got)
	}
	if c.DefinitionsSource() == "embedded" || c.ProjectionsSource() == "embedded" {
		t.Errorf("sources = %q, %q; want directories", c.DefinitionsSource(), c.ProjectionsSource())
	}

	e := catalogtest.Engine
	if got := typesOf(e.Definitions(t)); !slices.Equal(got, engineTypes) {
		t.Errorf("Engine.Definitions: %v", got)
	}
	if got := e.Projections(t).Names(); !slices.Equal(got, engineTemplates) {
		t.Errorf("Engine.Projections: %v", got)
	}
	if got := typesOf(e.Catalog(t).Definitions()); !slices.Equal(got, engineTypes) {
		t.Errorf("Engine.Catalog: %v", got)
	}
	first := []any{e.Catalog(t), e.Definitions(t), e.Projections(t)}
	if again := []any{e.Catalog(t), e.Definitions(t), e.Projections(t)}; !slices.Equal(first, again) {
		t.Error("an Engine helper built twice; want once per process")
	}
	var cfg config.Config
	e.Config(t, &cfg)
	fromCfg, err := catalog.Load(context.Background(), cfg.DefinitionsDir, cfg.ProjectionsDir)
	if err != nil {
		t.Fatal(err)
	}
	if got := fromCfg.SetVersion(); got != engineV {
		t.Errorf("catalog over Engine.Config's dirs: SetVersion = %s, want %s", got, engineV)
	}
}

// TestFixtures_AreDistinct checks Over builds one Fixture per catalog: Engine and a
// Fixture over fixture.Minimal share nothing, and each holds its own types and V.
func TestFixtures_AreDistinct(t *testing.T) {
	t.Parallel()
	over := catalogtest.Over(fixture.Minimal)
	if catalogtest.Engine.Catalog(t) == over.Catalog(t) ||
		catalogtest.Engine.Definitions(t) == over.Definitions(t) ||
		catalogtest.Engine.Projections(t) == over.Projections(t) {
		t.Error("Engine and Over(fixture.Minimal) share a helper's value")
	}
	if got := typesOf(over.Definitions(t)); !slices.Equal(got, []string{"record.document", "record.edge", "test.measure"}) {
		t.Errorf("Over(fixture.Minimal) types = %v", got)
	}
	if got := over.Catalog(t).SetVersion(); got == engineV || got == v {
		t.Errorf("Over(fixture.Minimal) SetVersion = %s, want Minimal's own", got)
	}
}

// TestEngine_PassesTheGuard checks the engine catalog's versions key and place facets
// as their history does: Guard accepts it on an empty store and over its own
// manifests, current and history, which holds the two widenings test.reading and
// test.templated make.
func TestEngine_PassesTheGuard(t *testing.T) {
	t.Parallel()
	defs, _ := fixture.Engine()
	reg := catalogtest.Engine.Definitions(t)
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
	history, err := def.HistoryFS(defs)
	if err != nil {
		t.Fatal(err)
	}
	if err := def.Guard(history, reg); err != nil {
		t.Errorf("Guard over the history alone: %v", err)
	}
	// Every history version is a contract that still parses, so a test can admit a
	// facet under it before a rename.
	for _, h := range history {
		if _, err := def.Parse(h.Body); err != nil {
			t.Errorf("history %s v%d does not parse: %v", h.Type, h.Version, err)
		}
	}
}

// TestEngine_RecordTypesAreStructural checks the pointers the engine reads from
// record.edge and record.document are declared in shapes their consumers can read:
// from, to and about as lists of strings, kind with a non-empty enum, a fact time
// whose zone loads, and sha256 required of a document.
func TestEngine_RecordTypesAreStructural(t *testing.T) {
	t.Parallel()
	defs, _ := fixture.Engine()
	type prop struct {
		Type  string          `json:"type"`
		Enum  []string        `json:"enum"`
		Items json.RawMessage `json:"items"`
	}
	read := func(name string) (meta struct {
		TimeSemantics struct {
			FactTime def.TimeField `json:"fact_time"`
		} `json:"time_semantics"`
	}, schema struct {
		Required   []string        `json:"required"`
		Properties map[string]prop `json:"properties"`
	}) {
		t.Helper()
		raw, err := fs.ReadFile(defs, name)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Meta   any             `json:"meta"`
			Schema json.RawMessage `json:"schema"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		metaRaw, _ := json.Marshal(m.Meta)
		if err := json.Unmarshal(metaRaw, &meta); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(m.Schema, &schema); err != nil {
			t.Fatal(err)
		}
		return meta, schema
	}

	meta, edge := read("record.edge.json")
	for _, p := range []string{"from", "to", "about"} {
		var items struct {
			Type string `json:"type"`
		}
		got := edge.Properties[p]
		if err := json.Unmarshal(got.Items, &items); got.Type != "array" || err != nil || items.Type != "string" {
			t.Errorf("record.edge /%s is %+v, want an array of strings", p, got)
		}
	}
	if kind := edge.Properties["kind"]; kind.Type != "string" || len(kind.Enum) == 0 {
		t.Errorf("record.edge /kind is %+v, want a string with a non-empty enum", kind)
	}
	if _, err := time.LoadLocation(meta.TimeSemantics.FactTime.Zone); err != nil || meta.TimeSemantics.FactTime.Zone != "UTC" {
		t.Errorf("record.edge fact time zone %q (%v), want UTC", meta.TimeSemantics.FactTime.Zone, err)
	}

	_, doc := read("record.document.json")
	if !slices.Contains(doc.Required, "sha256") || doc.Properties["sha256"].Type != "string" || doc.Properties["path"].Type != "string" {
		t.Errorf("record.document requires %v with sha256 %+v and path %+v; want sha256 required, both strings",
			doc.Required, doc.Properties["sha256"], doc.Properties["path"])
	}
}

// TestEngine_HoldsAFeatureEach checks each part of the engine catalog does what the
// tests that will read it need, so a converted test keeps the oracle it had.
func TestEngine_HoldsAFeatureEach(t *testing.T) {
	t.Parallel()
	reg := catalogtest.Engine.Definitions(t)
	lookup := func(typ string) def.Definition {
		t.Helper()
		d, ok := reg.Lookup(facet.Type(typ))
		if !ok {
			t.Fatalf("no %s in the engine catalog", typ)
		}
		return d
	}
	history := map[string]def.Definition{}
	for _, h := range catalogtest.Engine.Catalog(t).History() {
		d, err := def.Parse(h.Body)
		if err != nil {
			t.Fatal(err)
		}
		history[string(h.Type)] = d
	}

	t.Run("test.reading is read at the offset it names, or in its zone", func(t *testing.T) {
		t.Parallel()
		d := lookup("test.reading")
		if d.Kind != def.KindReading || d.FactTime.Zone != "Europe/Berlin" || d.FactTime.ZonePointer != "/source_time/offset" {
			t.Errorf("test.reading v%d: kind %q, fact time %+v", d.Version, d.Kind, d.FactTime)
		}
		if old := history["test.reading"]; old.Version != 1 || old.FactTime.ZonePointer != "" {
			t.Errorf("test.reading history v%d fact time %+v, want v1 with no zone pointer", old.Version, old.FactTime)
		}
		london := `{"id": "r1", "at": "2026-10-19 18:00", "value": 5, "unit": "u",
			"source_time": {"value": "2026-10-19 18:00", "zone": "Europe/London", "offset": "+01:00", "basis": "source"}}`
		at := factTime(t, d, london)
		if want := time.Date(2026, 10, 19, 17, 0, 0, 0, time.UTC); !at.Equal(want) {
			t.Errorf("a London reading is at %v, want %v", at, want)
		}
		if err := d.Check(decode(t, `{"id": "r1", "at": "2026-10-19 18:00", "value": 5, "unit": "u"}`)); err == nil {
			t.Error("v2 admits a reading with no source_time")
		}
		at, err := d.FactTimeOf(decode(t, `{"id": "r1", "at": "2026-10-19 18:00", "value": 5, "unit": "u"}`))
		if want := time.Date(2026, 10, 19, 16, 0, 0, 0, time.UTC); err != nil || !at.Equal(want) {
			t.Errorf("a reading with no zone is at %v, %v; want %v, in Berlin", at, err, want)
		}
		if got := d.Label([]byte(london)); got != "5 u" {
			t.Errorf("label %q, want \"5 u\"", got)
		}
	})

	t.Run("test.measure is keyed by two fields and its source_time is v1's shape", func(t *testing.T) {
		t.Parallel()
		d := lookup("test.measure")
		if !slices.Equal(d.IdentityKey, []string{"/at", "/seq"}) || d.Kind != def.KindMeasure ||
			d.Classification != facet.ClassificationNormal || d.FactTime.ZonePointer != "" {
			t.Errorf("test.measure v%d: key %v, kind %q, %s, fact time %+v", d.Version, d.IdentityKey, d.Kind, d.Classification, d.FactTime)
		}
		if old := history["test.measure"]; old.Version != 1 || d.Version != 2 {
			t.Errorf("test.measure v%d with history v%d, want v2 over v1", d.Version, old.Version)
		}
		ok := `{"at": "2026-10-19 08:00:00", "seq": 1, "count": 3,
			"source_time": {"value": "2026-10-19 08:00:00", "zone": "Europe/Berlin", "basis": "configuration"}}`
		if err := d.Check(decode(t, ok)); err != nil {
			t.Errorf("a measure is refused: %v", err)
		}
	})

	t.Run("test.templated composes its id, and v2 appends the offset", func(t *testing.T) {
		t.Parallel()
		d, old := lookup("test.templated"), history["test.templated"]
		if d.Classification != facet.ClassificationConfidential || !strings.HasPrefix(d.IdentityTemplate, old.IdentityTemplate) ||
			d.IdentityTemplate == old.IdentityTemplate {
			t.Errorf("test.templated v%d %s template %q over v%d %q", d.Version, d.Classification, d.IdentityTemplate, old.Version, old.IdentityTemplate)
		}
		plain := `{"at": "2026-10-17 22:00", "name": "Flat White", "amount": 1, "unit": "cup"}`
		for _, c := range []struct {
			d       def.Definition
			payload string
			want    string
		}{
			{old, plain, "workbench:templated/flat-white/2026-10-17T22:00"},
			{d, plain, "workbench:templated/flat-white/2026-10-17T22:00"},
			{d, strings.Replace(plain, `"unit": "cup"`, `"unit": "cup", "source_time": {"value": "2026-10-17 22:00", `+
				`"zone": "Europe/London", "offset": "+01:00", "basis": "source"}`, 1),
				"workbench:templated/flat-white/2026-10-17T22:00+01:00"},
		} {
			if got, err := def.ComposeID(c.d, "workbench", []byte(c.payload)); err != nil || got != c.want {
				t.Errorf("v%d composes %q, %v; want %q", c.d.Version, got, err, c.want)
			}
		}
		if got := d.Label([]byte(plain)); got != "Flat White 1 cup" {
			t.Errorf("label %q", got)
		}
		if _, err := def.ComposeID(lookup("test.reading"), "workbench", []byte(plain)); err == nil {
			t.Error("test.reading composes an id; want only test.templated to declare a template")
		}
	})

	t.Run("test.labelled renders the fields a payload has", func(t *testing.T) {
		t.Parallel()
		d := lookup("test.labelled")
		for payload, want := range map[string]string{
			`{"id": "l1", "at": "2026-10-17 08:00", "kind": "drink", "what": "flat-white"}`:  "drink flat-white",
			`{"id": "l2", "at": "2026-10-17 19:00", "kind": "other", "text": "words\nmore"}`: "other words",
		} {
			if got := d.Label([]byte(payload)); got != want {
				t.Errorf("label of %s = %q, want %q", payload, got, want)
			}
		}
	})

	t.Run("test.course is one series the stop closes, and so did its retired name", func(t *testing.T) {
		t.Parallel()
		for typ, closes := range map[facet.Type]bool{"test.course.started": false, "test.course.changed": false,
			"test.course.stopped": true, "test.retired": true, "test.measure": false} {
			if got := reg.Closes(typ); got != closes {
				t.Errorf("%s closes = %t, want %t", typ, got, closes)
			}
		}
		if reg.Has("test.retired") {
			t.Error("test.retired is in force; want it only in history")
		}
		stop, retired := lookup("test.course.stopped"), history["test.retired"]
		if stop.Series == nil || retired.Series == nil || stop.Series.Name != retired.Series.Name ||
			!slices.Equal(stop.IdentityKey, retired.IdentityKey) || stop.FactTime != retired.FactTime {
			t.Errorf("test.retired (v%d, series %+v, key %v, fact time %+v) does not key, place and series as "+
				"test.course.stopped (series %+v, key %v, fact time %+v) does",
				retired.Version, retired.Series, retired.IdentityKey, retired.FactTime, stop.Series, stop.IdentityKey, stop.FactTime)
		}
		for _, typ := range []string{"test.course.started", "test.course.changed"} {
			if s := lookup(typ).Series; s == nil || s.Name != "test.course" || !slices.Equal(s.Key, []string{"/name", "/course"}) {
				t.Errorf("%s series %+v", typ, s)
			}
		}
	})

	t.Run("test.pair is the bench's shape: two numbers and a time, keyed by the time", func(t *testing.T) {
		t.Parallel()
		d := lookup("test.pair")
		if !slices.Equal(d.IdentityKey, []string{"/at"}) || d.FactTime.Pointer != "/at" || d.IdentityTemplate != "" ||
			d.FactTime.ZonePointer != "" {
			t.Errorf("test.pair key %v, fact time %+v, template %q", d.IdentityKey, d.FactTime, d.IdentityTemplate)
		}
		if err := d.Check(decode(t, `{"at": "2026-10-19 08:00:00", "a": 120, "b": 80}`)); err != nil {
			t.Errorf("two numbers and a time are refused: %v", err)
		}
		if err := d.Check(decode(t, `{"at": "2026-10-19 08:00:00", "a": 120}`)); err == nil {
			t.Error("one number is admitted; want both required")
		}
	})

	t.Run("test.entry fits each course type by what it says", func(t *testing.T) {
		t.Parallel()
		entry := history["test.entry"]
		if entry.Version != 1 || reg.Has("test.entry") || entry.Series != nil {
			t.Fatalf("test.entry v%d, in force %t, series %+v; want v1 only in history, in no series",
				entry.Version, reg.Has("test.entry"), entry.Series)
		}
		withDose := `{"id": "start:1", "date": "2026-03-01", "name": "a", "course": 1, "dose": "1 u"}`
		without := `{"id": "stop:1", "date": "2026-03-09", "name": "a", "course": 1}`
		for _, c := range []struct {
			typ, payload string
			fits         bool
		}{
			{"test.course.started", withDose, true}, {"test.course.changed", withDose, true},
			{"test.course.stopped", without, true}, {"test.course.stopped", withDose, false},
			{"test.course.started", without, false},
		} {
			to := lookup(c.typ)
			if !slices.Equal(to.IdentityKey, entry.IdentityKey) || to.FactTime != entry.FactTime {
				t.Errorf("%s does not key and place as test.entry does", c.typ)
			}
			if err := entry.Check(decode(t, c.payload)); err != nil {
				t.Errorf("test.entry refuses %s: %v", c.payload, err)
			}
			if err := to.Check(decode(t, c.payload)); (err == nil) != c.fits {
				t.Errorf("%s takes %s: %v, want fits %t", c.typ, c.payload, err, c.fits)
			}
		}
	})

	t.Run("test.sample keys as test.reading and places by another rule", func(t *testing.T) {
		t.Parallel()
		sample, reading := history["test.sample"], lookup("test.reading")
		if reg.Has("test.sample") || !slices.Equal(sample.IdentityKey, reading.IdentityKey) || sample.FactTime == reading.FactTime {
			t.Fatalf("test.sample key %v fact time %+v; test.reading key %v fact time %+v",
				sample.IdentityKey, sample.FactTime, reading.IdentityKey, reading.FactTime)
		}
		london := `{"id": "s1", "at": "2026-10-19 18:00", "value": 5, "unit": "u",
			"source_time": {"value": "2026-10-19 18:00", "zone": "Europe/London", "offset": "+01:00", "basis": "source"}}`
		utc := strings.NewReplacer("+01:00", "+00:00", "2026-10-19", "2026-11-19").Replace(london)
		if factTime(t, sample, london).Equal(factTime(t, reading, london)) {
			t.Error("a London sample is placed alike by both; want test.reading to move it")
		}
		if !factTime(t, sample, utc).Equal(factTime(t, reading, utc)) {
			t.Error("a sample at +00:00 is placed apart; want it where it was")
		}
	})

	t.Run("every classification and kind is held", func(t *testing.T) {
		t.Parallel()
		classes, kinds := map[facet.Classification]bool{}, map[def.Kind]bool{}
		for _, d := range reg.All() {
			classes[d.Classification], kinds[d.Kind] = true, true
		}
		for _, c := range []facet.Classification{facet.ClassificationNormal, facet.ClassificationConfidential, facet.ClassificationSensitive} {
			if !classes[c] {
				t.Errorf("no %s type", c)
			}
		}
		for _, k := range []def.Kind{def.KindMeasure, def.KindReading, def.KindEvent} {
			if !kinds[k] {
				t.Errorf("no %s type", k)
			}
		}
	})

	t.Run("projections of each kind", func(t *testing.T) {
		t.Parallel()
		projs := catalogtest.Engine.Projections(t)
		get := func(name string, version int) projection.Template {
			t.Helper()
			tmpl, err := projs.Version(name, version)
			if err != nil {
				t.Fatal(err)
			}
			return tmpl
		}
		all := reg.All()
		if m := get("measures", 1); m.Shape != projection.ShapeLog || !slices.Equal(m.Covers(all), []facet.Type{"test.measure"}) {
			t.Errorf("measures: %s over %v", m.Shape, m.Covers(all))
		}
		course, was := get("course", 2), get("course", 1)
		if course.Shape != projection.ShapeSnapshot || was.Shape != projection.ShapeSnapshot ||
			!slices.Contains(was.Types, "test.retired") || slices.Contains(course.Types, "test.retired") {
			t.Errorf("course v2 %s %v over v1 %s %v", course.Shape, course.Types, was.Shape, was.Types)
		}
		if latest, err := projs.Lookup("course"); err != nil || latest.Version != 2 {
			t.Errorf("course resolves to v%d, %v; want v2", latest.Version, err)
		}
		every, normal := get("everything", 1), get("normal", 1)
		want := make([]facet.Type, 0, len(all))
		for _, d := range all {
			want = append(want, d.Type)
		}
		if !slices.Equal(every.Covers(all), want) || !slices.Contains(every.Prefixes, "test.course.") {
			t.Errorf("everything covers %v by %v, want every type with the series by its prefix", every.Covers(all), every.Patterns())
		}
		if every.MaxClassification != facet.ClassificationSensitive || normal.MaxClassification != facet.ClassificationNormal ||
			!slices.Equal(normal.Covers(all), want) {
			t.Errorf("everything ceiling %s, normal ceiling %s over %v", every.MaxClassification, normal.MaxClassification, normal.Covers(all))
		}
	})
}

func typesOf(r *def.Registry) []string {
	var out []string
	for _, d := range r.All() {
		out = append(out, string(d.Type))
	}
	return out
}

func decode(t *testing.T, payload string) any {
	t.Helper()
	doc, err := def.Decode([]byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func factTime(t *testing.T, d def.Definition, payload string) time.Time {
	t.Helper()
	doc := decode(t, payload)
	if err := d.Check(doc); err != nil {
		t.Fatalf("%s refuses %s: %v", d.Type, payload, err)
	}
	at, err := d.FactTimeOf(doc)
	if err != nil {
		t.Fatal(err)
	}
	return at
}
