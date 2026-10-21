package migrate

import (
	"sort"

	"github.com/mattjoyce/anam/internal/facet"
)

// Row counts the selection for one type and source: how many facets are carried, of
// those how many move to the type's current Definition version and how many keep the
// version they were admitted under, and how many the gate would refuse.
type Row struct {
	Type    facet.Type
	Source  string
	Moved   int
	Own     int
	Refused int
}

// Carry is how many facets of the row are carried.
func (r Row) Carry() int { return r.Moved + r.Own }

// Move is a group of facets carried under the current Definition version although they
// were admitted under an earlier one, for one type, source and pair of versions.
type Move struct {
	Type   facet.Type
	Source string
	From   int
	To     int
	N      int
}

// Kept is a group of facets that stay under an earlier version than the type's current
// one, because the current one does not give the same facet. Why is one example of the
// reason.
type Kept struct {
	Type    facet.Type
	Source  string
	Version int
	N       int
	Why     string
}

type groupKey struct {
	typ    facet.Type
	source string
}

// Tally counts the plan per type and source, in type then source order.
func (p Plan) Tally() []Row {
	rows := map[groupKey]*Row{}
	var keys []groupKey
	for _, it := range p.Items {
		k := groupKey{it.Facet.Type, it.Facet.Provenance.Source}
		r := rows[k]
		if r == nil {
			r = &Row{Type: k.typ, Source: k.source}
			rows[k] = r
			keys = append(keys, k)
		}
		switch {
		case it.Fate != FateCarry:
			r.Refused++
		case it.Moved:
			r.Moved++
		default:
			r.Own++
		}
	}
	sortKeys(keys)
	out := make([]Row, len(keys))
	for i, k := range keys {
		out[i] = *rows[k]
	}
	return out
}

// Moves lists the facets the plan carries under the current version, grouped by type,
// source and the version they came from.
func (p Plan) Moves() []Move {
	var moved []movedFacet
	for _, it := range p.Items {
		if it.Fate == FateCarry && it.Moved {
			moved = append(moved, movedFacet{it.Facet.Type, it.Facet.Provenance.Source, it.Facet.DefinitionVersion, it.Under})
		}
	}
	return groupMoves(moved)
}

// Kept lists the facets the plan carries under an earlier version than the current one
// because the current one would not give the same facet.
func (p Plan) Kept() []Kept {
	type key struct {
		groupKey
		version int
	}
	groups := map[key]*Kept{}
	var keys []key
	for _, it := range p.Items {
		if it.Fate != FateCarry || it.Moved || it.NotCurrent == "" {
			continue
		}
		k := key{groupKey{it.Facet.Type, it.Facet.Provenance.Source}, it.Facet.DefinitionVersion}
		g := groups[k]
		if g == nil {
			g = &Kept{Type: k.typ, Source: k.source, Version: k.version, Why: it.NotCurrent}
			groups[k] = g
			keys = append(keys, k)
		}
		g.N++
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].typ != keys[j].typ {
			return keys[i].typ < keys[j].typ
		}
		if keys[i].source != keys[j].source {
			return keys[i].source < keys[j].source
		}
		return keys[i].version < keys[j].version
	})
	out := make([]Kept, len(keys))
	for i, k := range keys {
		out[i] = *groups[k]
	}
	return out
}

type movedFacet struct {
	typ      facet.Type
	source   string
	from, to int
}

func groupMoves(moved []movedFacet) []Move {
	type key struct {
		groupKey
		from, to int
	}
	groups := map[key]*Move{}
	var keys []key
	for _, m := range moved {
		k := key{groupKey{m.typ, m.source}, m.from, m.to}
		g := groups[k]
		if g == nil {
			g = &Move{Type: m.typ, Source: m.source, From: m.from, To: m.to}
			groups[k] = g
			keys = append(keys, k)
		}
		g.N++
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].typ != keys[j].typ {
			return keys[i].typ < keys[j].typ
		}
		if keys[i].source != keys[j].source {
			return keys[i].source < keys[j].source
		}
		return keys[i].from < keys[j].from
	})
	out := make([]Move, len(keys))
	for i, k := range keys {
		out[i] = *groups[k]
	}
	return out
}

func sortKeys(keys []groupKey) {
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].typ != keys[j].typ {
			return keys[i].typ < keys[j].typ
		}
		return keys[i].source < keys[j].source
	})
}
