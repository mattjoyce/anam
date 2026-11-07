package inspect

import (
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
)

// The timeline's top level is the domain, the head of the dotted type (card 245). A
// domain is a lane: collapsed it draws every facet of its types on one line, opened it
// shows a row for each family beneath it and a sub-lane for each type, as the band did
// before. Colour follows the domain.

// domainOf is a type's head: the word before the first dot.
func domainOf(t facet.Type) string {
	head, _, _ := strings.Cut(string(t), ".")
	return head
}

// inClosedSet says whether a head is one of the ten.
func inClosedSet(id string) bool {
	switch id {
	case "health", "activity", "place", "record", "mind", "org", "money", "social", "media", "work":
		return true
	}
	return false
}

func (s styles) hueOf(t facet.Type) string { return s.presentation.Hue(domainOf(t)) }

// lane is one domain: its types in the ledger, and the family rows they make. The
// family rows are the same whether the lane is open or closed, so the legend does not
// move when a lane does.
type lane struct {
	ID       string
	Hue      string
	Families []tlRow
	Types    []facet.Type
}

// lanesFor orders eligible configured domains first, then unconfigured IDs
// lexically. Ledger heads are eligible regardless of vocabulary. Definitions alone
// make lanes only for recognized heads; configured retired names create no lane.
func (s styles) lanesFor(types []facet.Type) []lane {
	byDomain := map[string][]facet.Type{}
	present := map[string]bool{}
	for _, t := range types {
		d := domainOf(t)
		byDomain[d] = append(byDomain[d], t)
		present[d] = true
	}
	for _, d := range s.all() {
		if h := domainOf(d.Type); inClosedSet(h) {
			present[h] = true
		}
	}
	mk := func(id string) lane {
		ts := byDomain[id]
		return lane{ID: id, Hue: s.presentation.Hue(id), Families: s.rowsFor(ts), Types: ts}
	}
	var out []lane
	known := map[string]bool{}
	for _, d := range s.presentation.Domains() {
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
