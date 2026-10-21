package gate

import (
	"context"
	"fmt"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// A carry admits a facet that another ledger holds, as the facet it is (card 200): the
// same type, source, source ref, binding and payload, held to the Definition version it
// was admitted under, so its id, content hash and fact time come out as they were. It is
// the migration's one entry to a fresh ledger, and it is the gate's, not a second path
// to the tables: every pillar a commit runs runs here, against the version the facet
// carries instead of the type's current one, and the block is written by the same
// choreography as an admission.
//
// The concession is scoped by the call, as the rename's is. A commit is held to the
// Definition in force, because an envelope shaped to another version is refused; a
// carry is held to a version it names, and only a gate built with Holding can resolve
// one. Nothing on the wire reaches either.

// Held answers a Definition at a version: every version a ledger's facets were admitted
// under, not only the one in force. *def.Versions satisfies it.
type Held interface {
	// At is the Definition of type t at version, if it is held as a contract.
	At(t facet.Type, version int) (def.Definition, bool)
}

// Carry is one facet offered to a fresh ledger. Env carries the facet's type, source,
// injector, source ref and payload as stored; Binding is the facet's; DefinitionVersion
// is the version it was admitted under, which the carry is held to.
type Carry struct {
	Env               facet.Envelope
	Binding           facet.Binding
	DefinitionVersion int
}

// Holding is the gate with held as its answer to "this Definition at this version", the
// gate a carry runs on. The receiver is unchanged.
func (g *Gate) Holding(h Held) *Gate {
	c := *g
	c.held = h
	return &c
}

// pinned is the resolver of a carry: the Definition at the version the facet was
// admitted under.
func (g *Gate) pinned(version int) resolver {
	return func(env facet.Envelope) (def.Definition, error) {
		if g.held == nil {
			return def.Definition{}, fmt.Errorf("%w: this gate holds no Definition versions, so it cannot carry", ErrNoDefinition)
		}
		d, ok := g.held.At(env.Type, version)
		if !ok {
			return def.Definition{}, fmt.Errorf("%w: %s v%d is not held as a contract", ErrNoDefinition, env.Type, version)
		}
		return d, nil
	}
}

// VetCarry runs every pillar that needs no ledger read on one facet and answers the
// facet the gate would build, as the first version of itself, writing nothing and
// reading nothing. It is the migration's dry run: into a ledger with nothing in it
// every carry is a first admission, so these pillars are all that can refuse one. The
// caller compares the facet with the one it read, so a payload the gate would key,
// place or classify differently is a refusal before anything is written.
func (g *Gate) VetCarry(c Carry) (facet.Facet, error) {
	p, err := g.prepareCarry(c)
	if err != nil {
		return facet.Facet{}, err
	}
	return g.build(p, 1)
}

func (g *Gate) prepareCarry(c Carry) (prepared, error) {
	return g.prepareWith(c.Env, c.Binding, g.pinned(c.DefinitionVersion))
}

// CarryBatch commits carried facets as one block, as CommitBatch does envelopes: each
// facet as the first version of itself, or a no-op where the ledger already holds it
// byte for byte. A carry any pillar refuses refuses the block, and nothing is written.
func (g *Gate) CarryBatch(ctx context.Context, carries []Carry) ([]Result, ledger.Watermark, error) {
	if len(carries) == 0 {
		return nil, 0, nil
	}
	if len(carries) > MaxBatch {
		return nil, 0, fmt.Errorf("gate: batch of %d exceeds %d", len(carries), MaxBatch)
	}
	envs := make([]facet.Envelope, len(carries))
	for i, c := range carries {
		envs[i] = c.Env
	}
	ps, err := g.prepareEach(envs, func(i int) (prepared, error) { return g.prepareCarry(carries[i]) })
	if err != nil {
		return nil, 0, err
	}
	return g.commitPrepared(ctx, ps)
}
