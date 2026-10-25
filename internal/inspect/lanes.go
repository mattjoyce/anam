package inspect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
)

// The timeline's top level is the domain, the head of the dotted type (card 245). A
// domain is a lane: collapsed it draws every facet of its types on one line, opened it
// shows a row for each family beneath it and a sub-lane for each type, as the band did
// before. Colour follows the domain.

// domainInfo is a head of the closed set of ten (AGENTS.md) and its palette slot.
type domainInfo struct {
	ID  string
	Hue int // palette slot, 0..7, or hueNeutral
}

// domains in lane order, top to bottom. Seven are in use and take a hue of the eight;
// the three reserved heads have no type yet and take the grey until a decision gives
// them a hue, and have no lane until a type exists.
var domains = []domainInfo{
	{"health", 7},
	{"activity", 3},
	{"place", 1},
	{"record", 0},
	{"mind", 6},
	{"org", 5},
	{"money", 2},
	{"social", hueNeutral},
	{"media", hueNeutral},
	{"work", hueNeutral},
}

// domainOf is a type's head: the word before the first dot.
func domainOf(t facet.Type) string {
	head, _, _ := strings.Cut(string(t), ".")
	return head
}

// domainHue is the CSS class that colours a domain's marks; a head outside the
// palette takes the grey.
func domainHue(id string) string {
	for _, d := range domains {
		if d.ID == id && d.Hue != hueNeutral {
			return fmt.Sprintf("h%d", d.Hue)
		}
	}
	return "hx"
}

// hueOf is the CSS class that colours a type's marks.
func hueOf(t facet.Type) string { return domainHue(domainOf(t)) }

// lane is one domain: its types in the ledger, and the family rows they make. The
// family rows are the same whether the lane is open or closed, so the legend does not
// move when a lane does.
type lane struct {
	ID       string
	Hue      string
	Families []tlRow
	Types    []facet.Type
}

// lanesFor lays out the domains: every head with a type in the ledger or in the
// Definitions, in the order of domains, then a head outside the closed set that a
// ledger type carries (the table, not the ledger, would be wrong), sorted. A head with
// no type has no lane.
func (s styles) lanesFor(types []facet.Type) []lane {
	byDomain := map[string][]facet.Type{}
	present := map[string]bool{}
	for _, t := range types {
		d := domainOf(t)
		byDomain[d] = append(byDomain[d], t)
		present[d] = true
	}
	for _, d := range s.all() {
		present[domainOf(d.Type)] = true
	}
	mk := func(id string) lane {
		ts := byDomain[id]
		return lane{ID: id, Hue: domainHue(id), Families: s.rowsFor(ts), Types: ts}
	}
	var out []lane
	known := map[string]bool{}
	for _, d := range domains {
		known[d.ID] = true
		if present[d.ID] {
			out = append(out, mk(d.ID))
		}
	}
	var extra []string
	for id := range present {
		if !known[id] {
			extra = append(extra, id)
		}
	}
	sort.Strings(extra)
	for _, id := range extra {
		out = append(out, mk(id))
	}
	return out
}

// layoutLanes places each lane's rows. A lane is a row of its own; when it is open its
// family rows follow, each with a sub-lane for every type, a few units apart so a
// day's body-composition and weight do not land on the same spot. A closed lane's
// types all sit on the lane's own line. It returns the rows and the y of each type.
func layoutLanes(lanes []lane, isOpen func(string) bool) ([]tlRow, map[facet.Type]float64) {
	var rows []tlRow
	yOf := map[facet.Type]float64{}
	place := func(r tlRow) int {
		i := len(rows)
		r.Top, r.Height = rowTop+float64(i)*rowH, rowH
		r.Y = r.Top + rowH/2
		r.Stripe = i%2 == 0
		rows = append(rows, r)
		return i
	}
	for _, l := range lanes {
		open := isOpen(l.ID)
		head := tlRow{Domain: l.ID, Label: l.ID, Hue: l.Hue, Level: 0, Open: open}
		if !open {
			head.Types = l.Types
		}
		i := place(head)
		if !open {
			for _, t := range l.Types {
				yOf[t] = rows[i].Y
			}
			continue
		}
		for _, f := range l.Families {
			f.Domain, f.Level = l.ID, 1
			j := place(f)
			n := len(f.Types)
			step := 0.0
			if n > 1 {
				step = min(7, 18/float64(n-1))
			}
			for k, t := range f.Types {
				yOf[t] = rows[j].Y + (float64(k)-float64(n-1)/2)*step
			}
		}
	}
	return rows, yOf
}
