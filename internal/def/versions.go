package def

import (
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
)

// Versions is every Definition a set of bodies holds as a contract, by type and
// version: the ones in force, and the earlier versions a ledger's facets were admitted
// under (card 200). The gate admits only against the version in force; a migration
// carries a facet against the version it was admitted under, which is here.
//
// A body that no longer parses is not a contract. The history the binary carries is
// words, kept verbatim, and the lint of today need not have held then (History), so a
// version whose words do not parse today is reported as unreadable, with the reason,
// and not guessed at.
type Versions struct {
	held       map[versionKey]Definition
	unreadable map[versionKey]error
}

type versionKey struct {
	typ     facet.Type
	version int
}

// NewVersions holds the Definitions of current, then every body in bodies that parses.
// A version held twice is held once: the one in force wins, then the first body that
// parses.
func NewVersions(current *Registry, bodies ...[]Stored) *Versions {
	v := &Versions{held: map[versionKey]Definition{}, unreadable: map[versionKey]error{}}
	for _, d := range current.All() {
		v.held[versionKey{d.Type, d.Version}] = d
	}
	for _, set := range bodies {
		for _, s := range set {
			k := versionKey{s.Type, s.Version}
			if _, ok := v.held[k]; ok {
				continue
			}
			d, err := Parse(s.Body)
			switch {
			case err != nil:
				v.unreadable[k] = err
			case d.Type != s.Type || d.Version != s.Version:
				v.unreadable[k] = fmt.Errorf("%w: the body is %s v%d, not %s v%d", ErrBadManifest, d.Type, d.Version, s.Type, s.Version)
			default:
				v.held[k] = d
			}
		}
	}
	return v
}

// At is the Definition of type t at version, if it is held as a contract.
func (v *Versions) At(t facet.Type, version int) (Definition, bool) {
	d, ok := v.held[versionKey{t, version}]
	return d, ok
}

// Why says why t at version is not held: its words do not parse, or nothing holds them.
func (v *Versions) Why(t facet.Type, version int) string {
	k := versionKey{t, version}
	if err, ok := v.unreadable[k]; ok && v.held[k].Type == "" {
		return fmt.Sprintf("%s v%d is held as words that do not parse as a contract: %v", t, version, err)
	}
	return fmt.Sprintf("no Definition of %s at v%d is held", t, version)
}
