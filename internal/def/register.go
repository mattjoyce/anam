package def

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
)

// The ways a registration is refused. Each leaves the registered set as it was.
var (
	// ErrBundled: the type ships with this binary. A type's Definitions live in one
	// place; an importer takes a type over once the binary stops carrying it.
	ErrBundled = errors.New("def: type is bundled with this binary")

	// ErrRegistered: a different body is already registered at this type and version.
	// A registered version is immutable; a change is a new version.
	ErrRegistered = errors.New("def: a different body holds this version")

	// ErrVersionBehind: the version is older than one already held for the type.
	// Versions only go forward.
	ErrVersionBehind = errors.New("def: version is behind the newest held for the type")
)

// Registration is what registering a manifest decided.
type Registration struct {
	Definition Definition
	// New is false when the same body is already registered at this version, and
	// registering it again changes nothing.
	New bool
}

// Register decides whether body may be registered beside what is held: stored, the
// bodies kept for the bundled Definitions, every version seen; registered, every
// version registered before; bundled, this binary's own. It writes nothing.
//
// The same body at a held version is unchanged, or adopted when only the bundled
// copy held it: a type an importer takes over from the binary keeps its version.
// A new version must be newer than every one held, and must key and place facets as
// the newest does (Guard's rule).
func Register(body []byte, stored, registered []Stored, bundled *Registry) (Registration, error) {
	d, err := Parse(body)
	if err != nil {
		return Registration{}, err
	}
	if _, ok := bundled.Lookup(d.Type); ok {
		return Registration{}, fmt.Errorf("%w: %s", ErrBundled, d.Type)
	}
	canon, err := facet.Canonicalize(body)
	if err != nil {
		return Registration{}, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}

	reg, newest, found, err := againstHeld(d, canon, stored, registered)
	if err != nil || found {
		return reg, err
	}
	if newest == nil {
		return Registration{Definition: d, New: true}, nil
	}
	if d.Version < newest.Version {
		return Registration{}, fmt.Errorf("%w: %s v%d, newest is v%d", ErrVersionBehind, d.Type, d.Version, newest.Version)
	}
	old, err := keyingOf(newest.Body)
	if err != nil {
		return Registration{}, fmt.Errorf("def: held definition %s v%d: %w", newest.Type, newest.Version, err)
	}
	if err := sameKeying(old, keying{IdentityKey: d.IdentityKey, FactTime: d.FactTime}); err != nil {
		return Registration{}, fmt.Errorf("%w: %s v%d → v%d: %v", ErrKeyed, d.Type, newest.Version, d.Version, err)
	}
	return Registration{Definition: d, New: true}, nil
}

// againstHeld looks for d's own version among what is held. found says it is there,
// with the same body: unchanged when registered, adopted when only bundled. Otherwise
// newest is the newest version held of d's type, if any, for the caller to check d
// against. A different body at d's version is refused.
func againstHeld(d Definition, canon facet.Canonical, stored, registered []Stored) (Registration, *Stored, bool, error) {
	type held struct {
		Stored
		registered bool
	}
	all := make([]held, 0, len(registered)+len(stored))
	for _, s := range registered {
		all = append(all, held{s, true})
	}
	for _, s := range stored {
		all = append(all, held{s, false})
	}

	var newest *Stored
	for i := range all {
		h := &all[i]
		if h.Type != d.Type {
			continue
		}
		if h.Version == d.Version {
			same, err := sameBody(canon, h.Body)
			if err != nil {
				return Registration{}, nil, false, fmt.Errorf("def: held definition %s v%d: %w", h.Type, h.Version, err)
			}
			if !same {
				return Registration{}, nil, false, fmt.Errorf("%w: %s v%d", ErrRegistered, d.Type, d.Version)
			}
			// Registered rows come first, so a body both registered and bundled is
			// found registered: unchanged, not adopted twice.
			return Registration{Definition: d, New: !h.registered}, nil, true, nil
		}
		if newest == nil || h.Version > newest.Version {
			newest = &h.Stored
		}
	}
	return Registration{}, newest, false, nil
}

// sameBody compares two manifests as canonical JSON, so a file reformatted in the
// importer's repo is still the body it registered.
func sameBody(canon facet.Canonical, held []byte) (bool, error) {
	other, err := facet.Canonicalize(held)
	if err != nil {
		return false, err
	}
	return bytes.Equal(canon.Bytes(), other.Bytes()), nil
}

// WithRegistered is the registry the gate admits importers' envelopes against: the
// bundled Definitions, and the newest registered version of every other type.
//
// A type both bundled and registered is a release that took a type back from an
// importer. The bundled Definition wins: Register refuses the type from then on, and
// its registered rows stay held, immutable, for the facets admitted under them.
func WithRegistered(bundled *Registry, registered []Stored) (*Registry, error) {
	newest := map[facet.Type]Stored{}
	for _, s := range registered {
		if _, taken := bundled.byType[s.Type]; taken {
			continue
		}
		if cur, ok := newest[s.Type]; !ok || s.Version > cur.Version {
			newest[s.Type] = s
		}
	}
	defs := bundled.All()
	for _, s := range newest {
		d, err := Parse(s.Body)
		if err != nil {
			return nil, fmt.Errorf("def: registered %s v%d: %w", s.Type, s.Version, err)
		}
		defs = append(defs, d)
	}
	return NewRegistry(defs...)
}
