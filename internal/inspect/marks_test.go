package inspect

import (
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// Every bundled Definition has a place on the band. A type left out would still
// draw, as a grey circle in the "other" row, but that is a fallback for a type
// added in a hurry, not where a shipped type should live.
func TestTypeStyles_CoverEveryBundledType(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	entries, err := os.ReadDir("../def/manifests")
	if err != nil {
		t.Fatalf("read manifests: %v", err)
	}
	for _, e := range entries {
		typ := facet.Type(strings.TrimSuffix(e.Name(), ".json"))
		if _, ok := reg.Lookup(typ); !ok {
			t.Fatalf("%s is not a bundled type", typ)
		}
		if _, ok := typeStyles[typ]; !ok {
			t.Errorf("%s has no style in typeStyles", typ)
		}
	}
}

// The legend reads the shapes as a key: a filled circle is a measure, and an event
// is never a circle. (A reading may be one; it is drawn hollow.) Within a row no
// two types share a shape, except measures: a row may carry the same measurement
// from two eras (health.heart-rate from health-db, health.heart-rate-daily from
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

// The types the Garmin importer registers when health-db retired (card 55) sit in
// the rows of the types they replaced, not in "other": the live record is read
// in the rows, and the retired types still draw there in a read as_of an
// earlier watermark.
func TestTypeStyles_GarminTypesTakeTheRowsTheyReplaced(t *testing.T) {
	t.Parallel()
	cases := []struct {
		typ, replaced facet.Type
	}{
		{"health.heart-rate-daily", "health.heart-rate"},
		{"sleep.episode", "health.sleep"},
		{"health.step-count", "health.metric-group"},
		{"garmin.day", "health.metric-group"},
	}
	for _, c := range cases {
		got, old := styleOf(c.typ), styleOf(c.replaced)
		if got.Family == otherFamilyID {
			t.Errorf("%s falls into the other row", c.typ)
		}
		if got.Family != old.Family {
			t.Errorf("%s is in the %s row, %s in %s", c.typ, got.Family, c.replaced, old.Family)
		}
		if got.Kind != kindMeasure || got.Shape != shapeCircle {
			t.Errorf("%s: want a measure drawn as a circle, got %s/%d", c.typ, got.Shape, got.Kind)
		}
	}
}

// A shape means one kind of thing in every row, not only within one. The alert
// shape is for alerts alone, and every alert takes it: a heart rate alert once
// drew as a large red triangle and read as a trip beside travel's orange one.
// Starts and ends keep their triangles wherever they sit.
func TestTypeStyles_OneShapeMeansOneThingAcrossRows(t *testing.T) {
	t.Parallel()
	meaning := map[shape]func(facet.Type) bool{
		shapeAlert:   func(typ facet.Type) bool { return strings.HasSuffix(string(typ), "-alert") },
		shapeTriUp:   func(typ facet.Type) bool { return hasAnySuffix(typ, ".started", ".departed") },
		shapeTriDown: func(typ facet.Type) bool { return hasAnySuffix(typ, ".stopped", ".ended", ".arrived") },
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

// Each family keeps one palette slot of the eight, never shared. A family may
// instead take the neutral grey.
func TestFamilies_HuesAreDistinct(t *testing.T) {
	t.Parallel()
	seen := map[int]string{}
	for _, f := range families {
		if f.Hue == hueNeutral {
			continue
		}
		if f.Hue < 0 || f.Hue > 7 {
			t.Errorf("%s: hue %d is outside the eight palette slots", f.ID, f.Hue)
		}
		if other, dup := seen[f.Hue]; dup {
			t.Errorf("%s and %s share hue %d", f.ID, other, f.Hue)
		}
		seen[f.Hue] = f.ID
	}
}

func TestTypeFilter(t *testing.T) {
	t.Parallel()
	held := []facet.Type{"health.heart-rate", "health.heart-rate-alert", "lab.result"}
	cases := []struct {
		name     string
		q        []string
		show     []facet.Type
		selected int
	}{
		{"none named shows all", nil, held, 0},
		{"one named", []string{"health.heart-rate-alert"}, []facet.Type{"health.heart-rate-alert"}, 1},
		{"unknown names are dropped", []string{"no.such", "lab.result"}, []facet.Type{"lab.result"}, 1},
		{"only unknown names shows all", []string{"no.such"}, held, 0},
		{"all named is no filter", []string{"lab.result", "health.heart-rate", "health.heart-rate-alert"}, held, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			show, selected := typeFilter(tc.q, held)
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
