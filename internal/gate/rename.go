package gate

import (
	"context"
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// A rename carries a facet into another type as the next version of itself (card
// 140): the same id, the same source and identity values, the payload as it was or
// reshaped, under the type a transform table names. Anam-time is kept, because the
// old version keeps its seq and the new one carries the run's moment, so a read
// as-of before the run is unchanged and a read after shows the new type.
//
// The gate's one concession is scoped by the call. A commit that would change a
// facet's type is refused (card 105), and the content hash is over the payload
// alone, so a renamed facet whose payload did not change would read as a no-op.
// The two entry points here mark each prepared envelope with the type the facet is
// leaving, and disposition supersedes on a type change only when the facet's
// current type is that one. Nothing on the wire sets the mark, so the API's refusal
// stands, with its tests beside this file's.

// Rename is one facet carried into another type. Env names the type it arrives as
// and carries the facet's source and payload; Binding is the facet's, as stored;
// From is the type the facet is leaving, which its current version must hold.
type Rename struct {
	Env     facet.Envelope
	Binding facet.Binding
	From    facet.Type
}

// Rehearse decides what RenameBatch would do for one facet and writes nothing:
// every pillar a commit runs, then the disposition against the ledger as it stands.
// It is the dry run's question: would the gate take this, and as what? A facet the
// ledger does not hold answers admitted, which a rename run reads as an anomaly,
// since it carries only facets the fold listed.
func (g *Gate) Rehearse(ctx context.Context, r Rename) (Result, error) {
	p, err := g.prepare(r.Env, r.Binding)
	if err != nil {
		return Result{}, err
	}
	p.renamedFrom = r.From
	d, err := g.disposition(ctx, p)
	if err != nil {
		return Result{}, err
	}
	out := Result{FacetID: p.facetID, ArtifactVersion: d.version}
	switch {
	case d.noop:
		out.Outcome = OutcomeNoOp
	case d.kind == facet.EventSuperseded:
		out.Outcome = OutcomeSuperseded
	default:
		out.Outcome = OutcomeAdmitted
	}
	return out, nil
}

// RenameBatch commits renames as one block, as CommitBatch does envelopes: each
// facet's next version under its new type, or a no-op where it already arrived. A
// facet whose current type is not the one its rename says it is leaving is refused
// with ErrTypeChanged, and the block is not written.
func (g *Gate) RenameBatch(ctx context.Context, renames []Rename) ([]Result, ledger.Watermark, error) {
	if len(renames) == 0 {
		return nil, 0, nil
	}
	if len(renames) > MaxBatch {
		return nil, 0, fmt.Errorf("gate: batch of %d exceeds %d", len(renames), MaxBatch)
	}
	envs := make([]facet.Envelope, len(renames))
	bindings := make([]facet.Binding, len(renames))
	for i, r := range renames {
		envs[i], bindings[i] = r.Env, r.Binding
	}
	ps, err := g.prepareAll(envs, bindings)
	if err != nil {
		return nil, 0, err
	}
	for i := range ps {
		ps[i].renamedFrom = renames[i].From
	}
	return g.commitPrepared(ctx, ps)
}
