package def

import (
	"fmt"
	"slices"

	"github.com/mattjoyce/anam/internal/facet"
)

// Registry holds the loaded Definitions the gate consults. R3 builds registries from
// parsed manifests directly; bundling via go:embed and `anam def load` arrive with
// the first real capture worker (R5).
type Registry struct {
	byType map[facet.Type]Definition
}

// NewRegistry collects definitions, rejecting duplicate types: two contracts for one
// type is a configuration error, not a precedence question. It also refuses a series
// family declared with two different keys: the types of one family must stamp the
// same series for the same thing, or a stop could never close what a start opened.
func NewRegistry(defs ...Definition) (*Registry, error) {
	r := &Registry{byType: make(map[facet.Type]Definition, len(defs))}
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
// no Definition, or no series, closes nothing.
func (r *Registry) Closes(t facet.Type) bool {
	d, ok := r.byType[t]
	return ok && d.Series != nil && d.Series.Closes
}

// Lookup returns the Definition for a type.
func (r *Registry) Lookup(t facet.Type) (Definition, bool) {
	d, ok := r.byType[t]
	return d, ok
}
