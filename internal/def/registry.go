package def

import (
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
)

// Registry holds the loaded Definitions the gate consults. R3 builds registries from
// parsed manifests directly; bundling via go:embed and `anam def load` arrive with
// the first real capture worker (R5).
type Registry struct {
	byType map[facet.Type]Definition
}

// NewRegistry collects definitions, rejecting duplicate types: two contracts for one
// type is a configuration error, not a precedence question.
func NewRegistry(defs ...Definition) (*Registry, error) {
	r := &Registry{byType: make(map[facet.Type]Definition, len(defs))}
	for _, d := range defs {
		if _, dup := r.byType[d.Type]; dup {
			return nil, fmt.Errorf("%w: duplicate definition for type %q", ErrBadManifest, d.Type)
		}
		r.byType[d.Type] = d
	}
	return r, nil
}

// Lookup returns the Definition for a type.
func (r *Registry) Lookup(t facet.Type) (Definition, bool) {
	d, ok := r.byType[t]
	return d, ok
}
