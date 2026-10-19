package inspect

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// The timeline encodes a facet's type three ways, because nineteen types cannot
// be told apart by colour alone (dataviz: a ninth categorical hue is never
// generated). Each family gets a row of its own and one of the eight palette
// hues; within a row, each type gets a shape. Fill says what kind of fact it is:
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

// family is one row of the band.
type family struct {
	ID    string
	Label string
	Hue   int // palette slot, 0..7, or hueNeutral; the row's colour whatever else is on screen
}

// hueNeutral is the grey outside the eight palette slots. Documents take it: a
// library file is a reference, not something that happened to the person, and
// the eight hues are spent on the families that are.
const hueNeutral = -1

// families in row order, top to bottom. Hues are assigned once, here, and never
// move when a filter hides a family (dataviz: colour follows the entity).
var families = []family{
	{"heart", "heart", 7},
	{"body", "body", 0},
	{"sleep", "sleep", 6},
	{"activity", "activity", 3},
	{"labs", "labs", 2},
	{"meds", "meds", 4},
	{"clinical", "clinical", 5},
	{"travel", "travel", 1},
	{"documents", "documents", hueNeutral},
}

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

// typeStyles is every bundled or registered type's place on the band. A type not listed goes to
// the "other" row as a grey circle, so a new Definition shows up before anyone
// styles it.
var typeStyles = map[facet.Type]typeStyle{
	"health.heart-rate":       {"heart", shapeCircle, kindMeasure},
	"health.blood-pressure":   {"heart", shapeSquare, kindReading},
	"health.heart-rate-alert": {"heart", shapeAlert, kindEvent},
	// Registered by the Garmin importer since card 55, which retired health-db:
	// the day's heart rate that health.heart-rate was. Both are measures and both
	// are circles; the retired one draws only in a read as_of an earlier W.
	"health.heart-rate-daily": {"heart", shapeCircle, kindMeasure},

	"health.body-composition": {"body", shapeCircle, kindMeasure},
	"health.weight":           {"body", shapeSquare, kindReading},
	// Registered by the Withings importer since card 86, not bundled.
	"health.measurement": {"body", shapeDiamond, kindReading},

	"health.sleep": {"sleep", shapeCircle, kindMeasure},
	// One facet per CPAP session (card 70), registered by the DreamStation importer
	// rather than bundled: a session, drawn as the activity row draws one.
	"sleep.cpap-session": {"sleep", shapeDiamond, kindEvent},
	// A night of sleep from the Garmin importer (card 55), the successor of
	// health.sleep.
	"sleep.episode": {"sleep", shapeCircle, kindMeasure},

	"health.metric-group": {"activity", shapeCircle, kindMeasure},
	// The Garmin importer's day (card 55): a day's steps, and the day's summary
	// of floors, distance and calories. Successors of health.metric-group.
	"health.step-count": {"activity", shapeCircle, kindMeasure},
	"garmin.day":        {"activity", shapeCircle, kindMeasure},
	"gym.visit":         {"activity", shapeSquare, kindEvent},
	"exercise.session":  {"activity", shapeDiamond, kindEvent},

	"lab.result": {"labs", shapeCircle, kindReading},
	"lab.panel":  {"labs", shapeSquare, kindEvent},

	"med.compliance-estimate": {"meds", shapeCircle, kindMeasure},
	"med.started":             {"meds", shapeTriUp, kindEvent},
	"med.stopped":             {"meds", shapeTriDown, kindEvent},
	"med.dose-changed":        {"meds", shapeDiamond, kindEvent},
	"med.missed-dose":         {"meds", shapeCross, kindEvent},

	"visit.clinic":    {"clinical", shapeSquare, kindEvent},
	"condition.noted": {"clinical", shapeDiamond, kindEvent},
	"donation.blood":  {"clinical", shapeStar, kindEvent},
	"illness.started": {"clinical", shapeTriUp, kindEvent},
	"illness.ended":   {"clinical", shapeTriDown, kindEvent},
	// A lens prescription (card 36): a reading taken of the person, not a thing that
	// happened, so a hollow circle in the clinical row.
	"vision.prescription": {"clinical", shapeCircle, kindReading},

	"doc.reference": {"documents", shapeSquare, kindEvent},
	// A file Anam remembers but does not hold (card 165), registered by the links
	// importer: a document the library has no bytes for, so the documents row.
	"record.link": {"documents", shapeCross, kindEvent},
	// A note the person wrote about one or more facets: drawn with what is written
	// down, at the moment it was written (card 136); the facets it is about are
	// reached through its payload.
	"note.annotation": {"documents", shapeDiamond, kindEvent},
	// An hour of the phone's positions (card 130): a measure in the travel row,
	// which is where the person was; the eight hues are spent (card 133).
	"location.track":  {"travel", shapeCircle, kindMeasure},
	"travel.departed": {"travel", shapeTriUp, kindEvent},
	"travel.arrived":  {"travel", shapeTriDown, kindEvent},
	"travel.returned": {"travel", shapeDiamond, kindEvent},
	"travel.flight":   {"travel", shapeCross, kindEvent},
}

// styles resolves a type's place on the band for one render (card 176). A Definition
// that declares kind and family is read first, so a type the table never named draws
// on its family's row with its kind's marker, with no change to this file. A type
// the table names keeps its shape unless the Definition's kind disagrees. A type
// neither declares nor the table names goes to the "other" row as a grey circle.
type styles struct {
	lookup func(facet.Type) (def.Definition, bool)
}

// newStyles reads declared kind and family from reg. A nil registry declares
// nothing, so every type draws from the table as it did before card 176.
func newStyles(reg *def.Registry) styles {
	if reg == nil {
		return styles{lookup: func(facet.Type) (def.Definition, bool) { return def.Definition{}, false }}
	}
	return styles{lookup: reg.Lookup}
}

func (s styles) of(t facet.Type) typeStyle {
	st, listed := typeStyles[t]
	if !listed {
		st = typeStyle{Family: otherFamilyID, Shape: shapeCircle, Kind: kindMeasure}
	}
	d, ok := s.lookup(t)
	if !ok {
		return st
	}
	if d.Family != "" {
		st.Family = d.Family
	}
	if d.Kind == "" {
		return st
	}
	k := kindOf(d.Kind)
	if !listed || k != st.Kind {
		st.Kind, st.Shape = k, shapeFor(k)
	}
	return st
}

// isEvent says whether a type marks a thing that happened, rather than a measure
// or a reading. The "events only" filter keeps these.
func (s styles) isEvent(t facet.Type) bool { return s.of(t).Kind == kindEvent }

// markClass is the CSS class for a type's marks: its family's hue, hollow for a
// reading.
func (s styles) markClass(t facet.Type) string {
	st := s.of(t)
	if st.Kind == kindReading {
		return hueClass(st.Family) + " hollow"
	}
	return hueClass(st.Family)
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

// shapeFor is the marker a kind takes when the table names no shape for the type:
// the legend's key, a measure a filled circle, a reading a hollow one, an event a
// filled shape.
func shapeFor(k kind) shape {
	if k == kindEvent {
		return shapeDiamond
	}
	return shapeCircle
}

// hueClass is the CSS class that colours a family's marks.
func hueClass(familyID string) string {
	for _, f := range families {
		if f.ID == familyID && f.Hue != hueNeutral {
			return fmt.Sprintf("h%d", f.Hue)
		}
	}
	return "hx"
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

// rowsFor lays out the families present among types, in row order, with "other"
// last if anything fell into it. A family a Definition declares and the table does
// not know gets a row of its own, under its own word, after the known rows and in
// the neutral grey: the eight hues are assigned once, here, and a new family earns
// one by an edit to the table. Each row lists its types in a stable order: measures,
// then readings, then events, each by name.
func (s styles) rowsFor(types []facet.Type) []tlRow {
	byFamily := map[string][]facet.Type{}
	for _, t := range types {
		f := s.of(t).Family
		byFamily[f] = append(byFamily[f], t)
	}
	order := append([]family{}, families...)
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
		order = append(order, family{f, f, hueNeutral})
	}
	order = append(order, family{otherFamilyID, "other", hueNeutral})
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
		rows = append(rows, tlRow{Family: f.ID, Label: f.Label, Hue: hueClass(f.ID), Types: ts})
	}
	return rows
}
