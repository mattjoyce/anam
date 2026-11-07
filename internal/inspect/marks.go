package inspect

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/presentation"
)

// The timeline encodes a facet's type three ways, because nineteen types cannot
// be told apart by colour alone (dataviz: a ninth categorical hue is never
// generated). Each domain takes one of the eight palette hues, each family gets a row
// under its domain's lane; within a row, each type gets a shape. Fill says what kind of fact it is:
// a filled circle is a periodic measure (a day or a month of readings), a hollow
// mark is one reading from an instrument, and a filled shape is a thing that
// happened.

// shape is a mark's outline.
type shape string

const (
	shapeCircle   shape = "circle"
	shapeSquare   shape = "square"
	shapeDiamond  shape = "diamond"
	shapeTriUp    shape = "tri-up"
	shapeTriDown  shape = "tri-down"
	shapeCross    shape = "cross"
	shapeStar     shape = "star"
	shapeAlert    shape = "alert" // an exclamation mark drawn larger: the watch raising its voice
	markR               = 5.0
	markAlertR          = 7.5
	otherFamilyID       = "other"
)

// kind is what sort of fact a type records.
type kind int

const (
	kindMeasure kind = iota // a period's aggregate: a day of heart rate
	kindReading             // one instrument value: a blood pressure, a weight
	kindEvent               // a thing that happened: an alert, a dose change, a visit
)

type typeStyle struct {
	Family string
	Shape  shape
	Kind   kind
}

// styles combines a render's registry snapshot with immutable presentation policy.
// Definition metadata wins over legacy fallbacks; guarded shapes apply only when
// the effective kind agrees. Unknown types draw as measures in other.
type styles struct {
	presentation presentation.Value
	lookup       func(facet.Type) (def.Definition, bool)
	// all lists every Definition in force, so a domain with a type has a lane before
	// it has a facet (card 245).
	all func() []def.Definition
}

// newStyles captures the registry supplied for this render and one accepted presentation.
func newStyles(reg *def.Registry, p presentation.Value) styles {
	if reg == nil {
		return styles{presentation: p, lookup: func(facet.Type) (def.Definition, bool) { return def.Definition{}, false }, all: func() []def.Definition { return nil }}
	}
	return styles{presentation: p, lookup: reg.Lookup, all: reg.All}
}

func (s styles) of(t facet.Type) typeStyle {
	o, listed := s.presentation.Lookup(t)
	st := typeStyle{Family: otherFamilyID, Kind: kindMeasure}
	if o.LegacyFamily != "" {
		st.Family = o.LegacyFamily
	}
	if o.LegacyKind != "" {
		st.Kind = kindOf(def.Kind(o.LegacyKind))
	}
	if d, ok := s.lookup(t); ok {
		if d.Family != "" {
			st.Family = d.Family
		}
		if d.Kind != "" {
			st.Kind = kindOf(d.Kind)
		}
	}
	st.Shape = shapeFor(st.Kind)
	if listed && (o.LegacyKind == "" || kindOf(def.Kind(o.LegacyKind)) == st.Kind) {
		st.Shape = shape(o.Shape)
	}
	return st
}

// isEvent says whether a type marks a thing that happened, rather than a measure
// or a reading. The "events only" filter keeps these.
func (s styles) isEvent(t facet.Type) bool { return s.of(t).Kind == kindEvent }

// markClass is the CSS class for a type's marks: its domain's hue, hollow for a
// reading.
func (s styles) markClass(t facet.Type) string {
	st := s.of(t)
	if st.Kind == kindReading {
		return s.hueOf(t) + " hollow"
	}
	return s.hueOf(t)
}

// kindOf maps a Definition's declared kind onto the band's.
func kindOf(k def.Kind) kind {
	switch k {
	case def.KindReading:
		return kindReading
	case def.KindEvent:
		return kindEvent
	default:
		return kindMeasure
	}
}

// shapeFor is the marker a kind takes without an applicable override:
// the legend's key, a measure a filled circle, a reading a hollow one, an event a
// filled shape.
func shapeFor(k kind) shape {
	if k == kindEvent {
		return shapeDiamond
	}
	return shapeCircle
}

// markPath is the SVG path for a shape centred at (x, y).
func markPath(s shape, x, y float64) string {
	r := markR
	switch s {
	case shapeSquare:
		h := r * 0.85
		return fmt.Sprintf("M%.1f %.1fh%.1fv%.1fh%.1fz", x-h, y-h, 2*h, 2*h, -2*h)
	case shapeDiamond:
		d := r * 1.15
		return fmt.Sprintf("M%.1f %.1fl%.1f %.1fl%.1f %.1fl%.1f %.1fz", x, y-d, d, d, -d, d, -d, -d)
	case shapeAlert:
		// a bar over a dot, two subpaths in one path so it fills as one mark
		r = markAlertR
		w := r * 0.36
		return fmt.Sprintf("M%.1f %.1fh%.1fl%.1f %.1fh%.1fzM%.1f %.1fh%.1fv%.1fh%.1fz",
			x-w, y-r*1.1, 2*w, -w*0.4, r*1.25, -1.2*w,
			x-w*0.8, y+r*0.55, 1.6*w, r*0.5, -1.6*w)
	case shapeTriUp:
		return fmt.Sprintf("M%.1f %.1fL%.1f %.1fL%.1f %.1fz", x, y-r*1.1, x+r, y+r*0.75, x-r, y+r*0.75)
	case shapeTriDown:
		return fmt.Sprintf("M%.1f %.1fL%.1f %.1fL%.1f %.1fz", x, y+r*1.1, x+r, y-r*0.75, x-r, y-r*0.75)
	case shapeCross:
		a, w := r, r*0.35
		var b strings.Builder
		fmt.Fprintf(&b, "M%.1f %.1f", x-w, y-a)
		for _, p := range [][2]float64{{w, -a}, {w, -w}, {a, -w}, {a, w}, {w, w}, {w, a}, {-w, a}, {-w, w}, {-a, w}, {-a, -w}, {-w, -w}} {
			fmt.Fprintf(&b, "L%.1f %.1f", x+p[0], y+p[1])
		}
		b.WriteString("z")
		return b.String()
	case shapeStar:
		var b strings.Builder
		for i := range 10 {
			rr := r * 1.25
			if i%2 == 1 {
				rr = r * 0.55
			}
			a := math.Pi/2 - float64(i)*math.Pi/5
			cmd := "L"
			if i == 0 {
				cmd = "M"
			}
			fmt.Fprintf(&b, "%s%.1f %.1f", cmd, x+rr*math.Cos(a), y-rr*math.Sin(a))
		}
		b.WriteString("z")
		return b.String()
	default:
		return fmt.Sprintf("M%.1f %.1fa%.1f %.1f 0 1 0 %.1f 0a%.1f %.1f 0 1 0 %.1f 0z", x-r, y, r, r, 2*r, r, r, -2*r)
	}
}

// rowsFor orders configured families first, unconfigured IDs lexically, then other.
// Types sort by effective kind then ID. Every row takes its domain's hue.
func (s styles) rowsFor(types []facet.Type) []tlRow {
	byFamily := map[string][]facet.Type{}
	for _, t := range types {
		f := s.of(t).Family
		byFamily[f] = append(byFamily[f], t)
	}
	order := s.presentation.Families()
	known := map[string]bool{otherFamilyID: true}
	for _, f := range order {
		known[f.ID] = true
	}
	var declared []string
	for f := range byFamily {
		if !known[f] {
			declared = append(declared, f)
		}
	}
	sort.Strings(declared)
	for _, f := range declared {
		order = append(order, presentation.Family{ID: f, Label: f})
	}
	order = append(order, presentation.Family{ID: otherFamilyID, Label: "other"})
	var rows []tlRow
	for _, f := range order {
		ts := byFamily[f.ID]
		if len(ts) == 0 {
			continue
		}
		sort.Slice(ts, func(i, j int) bool {
			ki, kj := s.of(ts[i]).Kind, s.of(ts[j]).Kind
			if ki != kj {
				return ki < kj
			}
			return ts[i] < ts[j]
		})
		rows = append(rows, tlRow{Family: f.ID, Label: f.Label, Hue: s.hueOf(ts[0]), Types: ts})
	}
	return rows
}
