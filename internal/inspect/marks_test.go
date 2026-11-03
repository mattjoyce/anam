package inspect

import (
	"strings"
	"testing"
	"unicode"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// The legend reads the shapes as a key: a filled circle is a measure, and an event
// is never a circle. (A reading may be one; it is drawn hollow.) Within a row no
// two types share a shape, except measures: a row may carry the same measurement
// from two eras (health.heart-rate from health-db, health.heart-rate-day from
// Garmin), both circles, told apart by the legend's name and the row's sub-lanes.
func TestTypeStyles_ShapesMeanWhatTheLegendSays(t *testing.T) {
	t.Parallel()
	known := map[string]bool{}
	for _, f := range families {
		known[f.ID] = true
	}
	inRow := map[[2]string]facet.Type{}
	for typ, st := range typeStyles {
		if !known[st.Family] {
			t.Errorf("%s: family %q is not a row", typ, st.Family)
		}
		if st.Kind == kindEvent && st.Shape == shapeCircle {
			t.Errorf("%s: an event drawn as a circle would read as a measure", typ)
		}
		if st.Kind == kindMeasure && st.Shape != shapeCircle {
			t.Errorf("%s: a measure must be a circle, got %s", typ, st.Shape)
		}
		if st.Kind == kindMeasure {
			continue
		}
		key := [2]string{st.Family, string(st.Shape)}
		if other, dup := inRow[key]; dup {
			t.Errorf("%s and %s share the %s shape in the %s row", typ, other, st.Shape, st.Family)
		}
		inRow[key] = typ
	}
}

// A shape means one kind of thing in every row, not only within one. The alert
// shape is for alerts alone, and every alert takes it: a heart rate alert once
// drew as a large red triangle and read as a trip beside travel's orange one.
// Starts and ends keep their triangles wherever they sit.
func TestTypeStyles_OneShapeMeansOneThingAcrossRows(t *testing.T) {
	t.Parallel()
	meaning := map[shape]func(facet.Type) bool{
		shapeAlert: func(typ facet.Type) bool { return strings.HasSuffix(string(typ), "-alert") },
		shapeTriUp: func(typ facet.Type) bool { return hasAnySuffix(typ, "-started", ".started", ".departed") },
		shapeTriDown: func(typ facet.Type) bool {
			return hasAnySuffix(typ, "-stopped", "-ended", ".stopped", ".ended", ".arrived")
		},
	}
	for typ, st := range typeStyles {
		for sh, means := range meaning {
			if means(typ) && st.Shape != sh {
				t.Errorf("%s: want the %s shape, got %s", typ, sh, st.Shape)
			}
			if st.Shape == sh && !means(typ) {
				t.Errorf("%s: the %s shape means something %s is not", typ, sh, typ)
			}
		}
	}
}

// A Definition that declares kind and family draws on that family's row with the
// kind's marker, with no entry in the table (card 176). A declared family the table
// does not know is a row of its own, grey, after the known rows. A type the table
// names keeps its shape when the Definition agrees on its kind, and without a
// registry every type draws from the table as before.
func TestStyles_DeclaredKindAndFamilyWin(t *testing.T) {
	t.Parallel()
	manifest := func(typ, kind, family string) string {
		return `{"meta": {"facet_type": "` + typ + `", "definition_version": 1, "classification": "Normal",
		  "validation_mode": "strict", "source_schema": "test/row", "kind": "` + kind + `", "family": "` + family + `",
		  "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02", "zone": "UTC"}},
		  "identity_key": ["/id"]},
		 "schema": {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
		  "required": ["id", "at"],
		  "properties": {"id": {"type": "string", "minLength": 1}, "at": {"type": "string", "minLength": 1}},
		  "additionalProperties": false}}`
	}
	parse := func(m string) def.Definition {
		d, err := def.Parse([]byte(m))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		return d
	}
	reg, err := def.NewRegistry(
		parse(manifest("test.pressure", "reading", "vitals")),
		parse(manifest("test.harvest", "event", "garden")),
		parse(manifest("health.medication.started", "event", "meds")),
	)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	st := newStyles(reg)

	if got := st.of("test.pressure"); got.Family != "vitals" || got.Kind != kindReading || got.Shape != shapeCircle {
		t.Errorf("declared reading in vitals: got %+v", got)
	}
	if c := st.markClass("test.pressure"); !strings.HasSuffix(c, " hollow") {
		t.Errorf("a declared reading draws hollow, got class %q", c)
	}
	if got := st.of("test.harvest"); got.Family != "garden" || got.Kind != kindEvent || got.Shape != shapeDiamond || !st.isEvent("test.harvest") {
		t.Errorf("declared event in an unknown family: got %+v", got)
	}
	if got := st.of("health.medication.started"); got.Shape != shapeTriUp || got.Family != "meds" {
		t.Errorf("a table type whose Definition agrees keeps its shape, got %+v", got)
	}
	if got := newStyles(nil).of("test.pressure"); got.Family != otherFamilyID || got.Kind != kindMeasure {
		t.Errorf("without a registry the table decides, got %+v", got)
	}

	rows := st.rowsFor([]facet.Type{"test.harvest", "health.medication.started", "test.pressure", "no.such"})
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.Family)
		if r.Family == "garden" && (r.Label != "garden" || r.Hue != "hx") {
			t.Errorf("the garden row: label %q hue %q, want its own word in grey", r.Label, r.Hue)
		}
	}
	want := []string{"vitals", "meds", "garden", "other"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Errorf("rows %v, want %v: known families, then declared, then other", ids, want)
	}
}

func hasAnySuffix(typ facet.Type, suffixes ...string) bool {
	for _, s := range suffixes {
		if strings.HasSuffix(string(typ), s) {
			return true
		}
	}
	return false
}

// The alert's outline is its own: no other shape draws with the same sequence of
// path commands, which a larger copy of a triangle would.
func TestMarkPath_AlertOutlineIsItsOwn(t *testing.T) {
	t.Parallel()
	outline := func(s shape) string {
		return strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) {
				return r
			}
			return -1
		}, markPath(s, 0, 0))
	}
	alert := outline(shapeAlert)
	for _, s := range []shape{shapeCircle, shapeSquare, shapeDiamond, shapeTriUp, shapeTriDown, shapeCross, shapeStar} {
		if outline(s) == alert {
			t.Errorf("the alert draws the same outline as %s (%s)", s, alert)
		}
	}
}

// The ten heads are the closed set of AGENTS.md. Each domain in use keeps one palette
// slot of the eight, never shared; a reserved head takes the grey (card 245).
func TestDomains_TenHeadsAndHuesAreDistinct(t *testing.T) {
	t.Parallel()
	if len(domains) != 10 {
		t.Fatalf("%d heads, want the closed set of ten", len(domains))
	}
	seen := map[int]string{}
	grey := map[string]bool{"social": true, "media": true, "work": true}
	for _, d := range domains {
		if grey[d.ID] != (d.Hue == hueNeutral) {
			t.Errorf("%s: hue %d, but only the reserved heads %v take the grey", d.ID, d.Hue, grey)
		}
		if d.Hue == hueNeutral {
			continue
		}
		if d.Hue < 0 || d.Hue > 7 {
			t.Errorf("%s: hue %d is outside the eight palette slots", d.ID, d.Hue)
		}
		if other, dup := seen[d.Hue]; dup {
			t.Errorf("%s and %s share hue %d", d.ID, other, d.Hue)
		}
		seen[d.Hue] = d.ID
	}
}

func TestTypeFilter(t *testing.T) {
	t.Parallel()
	held := []facet.Type{"health.heart-rate", "health.heart-rate-alert", "health.lab-result"}
	cases := []struct {
		name     string
		q        []string
		show     []facet.Type
		selected int
		none     bool
	}{
		{"none named shows all", nil, held, 0, false},
		{"one named", []string{"health.heart-rate-alert"}, []facet.Type{"health.heart-rate-alert"}, 1, false},
		{"unknown names are dropped", []string{"no.such", "health.lab-result"}, []facet.Type{"health.lab-result"}, 1, false},
		{"only unknown names shows all", []string{"no.such"}, held, 0, false},
		{"all named is no filter", []string{"health.lab-result", "health.heart-rate", "health.heart-rate-alert"}, held, 0, false},
		{"none alone shows none", []string{"none"}, nil, 0, true},
		{"none beside a type shows the type", []string{"none", "health.lab-result"}, []facet.Type{"health.lab-result"}, 1, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			show, selected, none := typeFilter(tc.q, held)
			if none != tc.none {
				t.Errorf("none = %v, want %v", none, tc.none)
			}
			if len(show) != len(tc.show) {
				t.Errorf("show = %v, want %v", show, tc.show)
			}
			for _, typ := range tc.show {
				if !show[typ] {
					t.Errorf("%s hidden, want shown", typ)
				}
			}
			if len(selected) != tc.selected {
				t.Errorf("selected = %v, want %d", selected, tc.selected)
			}
		})
	}
}

// A record.link (card 165) is a document the library has no bytes for, registered
// by the links importer: it draws in the documents row, as an event, with a shape
// of its own there.
func TestTypeStyles_LinkDrawsInTheDocumentsRow(t *testing.T) {
	t.Parallel()
	st := newStyles(nil)
	got, ref := st.of("record.link"), st.of("record.document")
	if got.Family != ref.Family || got.Family == otherFamilyID {
		t.Errorf("record.link is in the %s row, record.document in %s", got.Family, ref.Family)
	}
	if got.Kind != kindEvent || got.Shape == ref.Shape {
		t.Errorf("record.link: want an event with its own shape, got %s/%d beside %s", got.Shape, got.Kind, ref.Shape)
	}
}

// A head of the closed set with a Definition has a lane before it has a facet, but a
// head outside the set has one only when the ledger holds a facet of it: an old name
// whose Definition is still held draws no empty lane, and its facets, if any, are not
// hidden (card 245).
func TestLanesFor_OnlyTheClosedSetIsDrawnFromDefinitions(t *testing.T) {
	t.Parallel()
	st := styles{
		lookup: func(facet.Type) (def.Definition, bool) { return def.Definition{}, false },
		all: func() []def.Definition {
			return []def.Definition{{Type: "money.purchase"}, {Type: "sleep.episode"}, {Type: "garmin.day"}, {Type: "location.stay"}}
		},
	}
	ids := func(types ...facet.Type) string {
		var out []string
		for _, l := range st.lanesFor(types) {
			out = append(out, l.ID)
		}
		return strings.Join(out, ",")
	}
	if got := ids(); got != "money" {
		t.Errorf("Definitions alone give lanes %q, want money only: sleep, garmin and location are no domains", got)
	}
	if got := ids("garmin.day", "health.weight"); got != "health,money,garmin" {
		t.Errorf("lanes %q, want the closed set first and the off-set head its facet makes", got)
	}
}
