package def

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/mattjoyce/anam/internal/facet"
)

// Registry holds the loaded Definitions the gate consults: the bundled ones, and
// those importers registered.
type Registry struct {
	byType map[facet.Type]Definition

	// retired holds the types the catalog's history names, mapped to whether their
	// newest history version closes a series; a type a Definition in force names is
	// answered from the Definition. Only Closes reads it: the gate admits nothing under
	// a retired type, but a replay of a read served before the type was renamed (card
	// 147) must still know that a stop closed its course (card 187). It comes from the
	// history of the catalog the registry was built over (BundledFS), never from one the
	// binary carries (card 255), and WithRegistered keeps the bundled registry's.
	retired map[facet.Type]bool
}

// NewRegistry collects definitions, rejecting duplicate types: two contracts for one
// type is a configuration error, not a precedence question. It also refuses a series
// family declared with two different keys: the types of one family must stamp the
// same series for the same thing, or a stop could never close what a start opened.
// A registry built here has no history, so no retired type closes; BundledFS gives
// one the history of its catalog.
func NewRegistry(defs ...Definition) (*Registry, error) {
	return newRegistry(nil, defs...)
}

// newRegistry is NewRegistry over a retired-type map taken from a catalog's history.
func newRegistry(retired map[facet.Type]bool, defs ...Definition) (*Registry, error) {
	r := &Registry{byType: make(map[facet.Type]Definition, len(defs)), retired: retired}
	families := map[string]Definition{}
	for _, d := range defs {
		if _, dup := r.byType[d.Type]; dup {
			return nil, fmt.Errorf("%w: duplicate definition for type %q", ErrBadManifest, d.Type)
		}
		r.byType[d.Type] = d
		if d.Series == nil {
			continue
		}
		if first, seen := families[d.Series.Name]; seen && !slices.Equal(first.Series.Key, d.Series.Key) {
			return nil, fmt.Errorf("%w: series %q is keyed %v by %s and %v by %s",
				ErrBadManifest, d.Series.Name, first.Series.Key, first.Type, d.Series.Key, d.Type)
		} else if !seen {
			families[d.Series.Name] = d
		}
	}
	return r, nil
}

// Closes reports whether a facet of type t ends the series it belongs to. A type with
// no Definition in force answers as its newest history version did, so a ledger read
// as-of before a rename still closes what it closed then (card 187); a type with no
// series, or with no Definition anywhere, closes nothing.
func (r *Registry) Closes(t facet.Type) bool {
	if d, ok := r.byType[t]; ok {
		return d.Series != nil && d.Series.Closes
	}
	return r.retired[t]
}

// retiredCloses reads, for every type a catalog's history carries, whether its newest
// history version closes a series. A history body is words, not a contract, so only
// the one field is read. Types a registry holds a Definition for are ignored at lookup,
// not here, so the answer does not depend on which Definitions the registry was given.
func retiredCloses(history []Stored) (map[facet.Type]bool, error) {
	newest := map[facet.Type]int{}
	out := map[facet.Type]bool{}
	for _, h := range history {
		if h.Version <= newest[h.Type] {
			continue
		}
		var m struct {
			Meta struct {
				Series *struct {
					Closes bool `json:"closes"`
				} `json:"series"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(h.Body, &m); err != nil {
			return nil, fmt.Errorf("%w: history %s v%d: %v", ErrBadManifest, h.Type, h.Version, err)
		}
		newest[h.Type] = h.Version
		out[h.Type] = m.Meta.Series != nil && m.Meta.Series.Closes
	}
	return out, nil
}

// Identify is the source_ref the gate admits an envelope under: its Definition's
// identity_key composed over the payload. It ignores any source_ref the envelope
// carries, which the gate only checks against this.
func (r *Registry) Identify(env facet.Envelope) (string, error) {
	d, ok := r.byType[env.Type]
	if !ok {
		return "", fmt.Errorf("%w: no definition for %s", ErrNotCompliant, env.Type)
	}
	payload, err := d.WithID(env.Source, env.Payload)
	if err != nil {
		return "", err
	}
	doc, err := Decode(payload)
	if err != nil {
		return "", err
	}
	return d.SourceRef(env.Source, doc)
}

// Has reports whether the registry holds a Definition for a type.
func (r *Registry) Has(t facet.Type) bool {
	_, ok := r.byType[t]
	return ok
}

// Lookup returns the Definition for a type.
func (r *Registry) Lookup(t facet.Type) (Definition, bool) {
	d, ok := r.byType[t]
	return d, ok
}
