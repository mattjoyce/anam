package inspect

import (
	"fmt"
	"math"
	"sort"
	"strings"

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
	shapeAlert    shape = "alert" // a triangle drawn larger: the watch raising its voice
	markR               = 5.0
	markAlertR          = 7.5
	otherFamilyID       = "other"
)

// family is one row of the band.
type family struct {
	ID    string
	Label string
	Hue   int // palette slot, 0..7; the row's colour whatever else is on screen
}

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
	{"documents", "docs & travel", 1},
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

// typeStyles is every bundled type's place on the band. A type not listed goes to
// the "other" row as a grey circle, so a new Definition shows up before anyone
// styles it.
var typeStyles = map[facet.Type]typeStyle{
	"health.heart-rate":       {"heart", shapeCircle, kindMeasure},
	"health.blood-pressure":   {"heart", shapeSquare, kindReading},
	"health.heart-rate-alert": {"heart", shapeAlert, kindEvent},

	"health.body-composition": {"body", shapeCircle, kindMeasure},
	"health.weight":           {"body", shapeSquare, kindReading},
	"health.measurement":      {"body", shapeDiamond, kindReading},

	"health.sleep": {"sleep", shapeCircle, kindMeasure},

	"health.metric-group": {"activity", shapeCircle, kindMeasure},
	"gym.visit":           {"activity", shapeSquare, kindEvent},

	"lab.result": {"labs", shapeCircle, kindReading},
	"lab.panel":  {"labs", shapeSquare, kindEvent},

	"med.compliance-estimate": {"meds", shapeCircle, kindMeasure},
	"med.started":             {"meds", shapeTriUp, kindEvent},
	"med.stopped":             {"meds", shapeTriDown, kindEvent},
	"med.dose-changed":        {"meds", shapeDiamond, kindEvent},
	"med.missed-dose":         {"meds", shapeCross, kindEvent},

	"visit.clinic":     {"clinical", shapeSquare, kindEvent},
	"condition.noted":  {"clinical", shapeDiamond, kindEvent},
	"donation.blood":   {"clinical", shapeStar, kindEvent},
	"illness.started":  {"clinical", shapeTriUp, kindEvent},
	"illness.ended":    {"clinical", shapeTriDown, kindEvent},
	"illness.hangover": {"clinical", shapeCross, kindEvent},

	"doc.reference":   {"documents", shapeSquare, kindEvent},
	"travel.departed": {"documents", shapeTriUp, kindEvent},
	"travel.arrived":  {"documents", shapeTriDown, kindEvent},
	"travel.returned": {"documents", shapeDiamond, kindEvent},
	"travel.flight":   {"documents", shapeCross, kindEvent},
}

func styleOf(t facet.Type) typeStyle {
	if s, ok := typeStyles[t]; ok {
		return s
	}
	return typeStyle{Family: otherFamilyID, Shape: shapeCircle, Kind: kindMeasure}
}

// isEvent says whether a type marks a thing that happened, rather than a measure
// or a reading. The "events only" filter keeps these.
func isEvent(t facet.Type) bool { return styleOf(t).Kind == kindEvent }

// markClass is the CSS class for a type's marks: its family's hue, hollow for a
// reading.
func markClass(t facet.Type) string {
	st := styleOf(t)
	if st.Kind == kindReading {
		return hueClass(st.Family) + " hollow"
	}
	return hueClass(st.Family)
}

// hueClass is the CSS class that colours a family's marks.
func hueClass(familyID string) string {
	for _, f := range families {
		if f.ID == familyID {
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
	case shapeTriUp, shapeAlert:
		if s == shapeAlert {
			r = markAlertR
		}
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
// last if anything fell into it. Each row lists its types in a stable order:
// measures, then readings, then events, each by name.
func rowsFor(types []facet.Type) []tlRow {
	byFamily := map[string][]facet.Type{}
	for _, t := range types {
		f := styleOf(t).Family
		byFamily[f] = append(byFamily[f], t)
	}
	order := append([]family{}, families...)
	order = append(order, family{otherFamilyID, "other", -1})
	var rows []tlRow
	for _, f := range order {
		ts := byFamily[f.ID]
		if len(ts) == 0 {
			continue
		}
		sort.Slice(ts, func(i, j int) bool {
			ki, kj := styleOf(ts[i]).Kind, styleOf(ts[j]).Kind
			if ki != kj {
				return ki < kj
			}
			return ts[i] < ts[j]
		})
		rows = append(rows, tlRow{Family: f.ID, Label: f.Label, Hue: hueClass(f.ID), Types: ts})
	}
	return rows
}
