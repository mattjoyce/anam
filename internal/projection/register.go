package projection

import (
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
)

// The ways a registration is refused. Each leaves the held set as it was.
var (
	// ErrRegistered: a different body is already held at this name and version. A
	// held version is immutable, bundled or registered; a change is a new version.
	ErrRegistered = errors.New("projection: a different body holds this version")

	// ErrVersionBehind: the version is older than the newest held for the name.
	// Versions only go forward.
	ErrVersionBehind = errors.New("projection: version is behind the newest held for the template")

	// ErrUndefinedType: the template names a type no Definition publishes. A
	// template naming a typo is refused at the door rather than serving nothing
	// quietly. A type whose Definition is later retired stays nameable: its past
	// facets are still servable.
	ErrUndefinedType = errors.New("projection: template names a type with no Definition")
)

// Stored is one template body the operational store holds, by name and version.
type Stored struct {
	Name    string
	Version int
	Body    []byte
}

// Registration is what registering a manifest decided.
type Registration struct {
	Template Template
	// New is false when the same body is already held at this version, bundled or
	// registered, and registering it again changes nothing.
	New bool
}

// Register decides whether body may be registered beside what held holds. defined
// says whether a type has a Definition in force; nil skips the check. It writes
// nothing.
//
// The same body at a held version is unchanged. A new version must be newer than
// every one held for the name, so a bundled template is the floor its registered
// successors build on, never something they rewrite.
func Register(body []byte, held *Registry, defined func(facet.Type) bool) (Registration, error) {
	t, err := Parse(body)
	if err != nil {
		return Registration{}, err
	}
	if defined != nil {
		for _, typ := range t.Types {
			if !defined(typ) {
				return Registration{}, fmt.Errorf("%w: %s names %s", ErrUndefinedType, t.Name, typ)
			}
		}
	}
	versions := held.Versions(t.Name)
	for _, h := range versions {
		if h.Version != t.Version {
			continue
		}
		if h.Hash == t.Hash {
			return Registration{Template: t}, nil
		}
		return Registration{}, fmt.Errorf("%w: %s v%d", ErrRegistered, t.Name, t.Version)
	}
	if n := len(versions); n > 0 && t.Version < versions[n-1].Version {
		return Registration{}, fmt.Errorf("%w: %s v%d, newest is v%d", ErrVersionBehind, t.Name, t.Version, versions[n-1].Version)
	}
	return Registration{Template: t, New: true}, nil
}

// WithRegistered is the registry reads select from and receipts verify against: every
// bundled template and every registered one, all versions. A registered body at a
// bundled name and version is the same template when the bodies agree; two bodies
// at one version is a store this binary cannot serve from, and it says so rather
// than picking one.
func WithRegistered(bundled *Registry, registered []Stored) (*Registry, error) {
	all := bundled.Held()
	for _, s := range registered {
		t, err := Parse(s.Body)
		if err != nil {
			return nil, fmt.Errorf("projection: registered %s v%d: %w", s.Name, s.Version, err)
		}
		if t.Name != s.Name || t.Version != s.Version {
			return nil, fmt.Errorf("%w: registered row %s v%d holds a body naming %s v%d",
				ErrBadTemplate, s.Name, s.Version, t.Name, t.Version)
		}
		all = append(all, t)
	}
	return NewRegistry(all...)
}
