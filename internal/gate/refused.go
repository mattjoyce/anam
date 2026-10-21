package gate

import (
	"fmt"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
)

// Refused is a batch the gate wrote nothing of, with every envelope that failed a
// pillar and why. errors.Is and errors.As see each envelope's own error through it.
type Refused struct {
	Envelopes []RefusedEnvelope
}

// RefusedEnvelope is one envelope of a refused batch, by its index in the batch.
type RefusedEnvelope struct {
	Index int
	Type  facet.Type
	Err   error
}

func (r *Refused) Error() string {
	msgs := make([]string, len(r.Envelopes))
	for i, e := range r.Envelopes {
		msgs[i] = fmt.Sprintf("envelope %d (%s): %v", e.Index, e.Type, e.Err)
	}
	return strings.Join(msgs, "; ")
}

// Unwrap is every refused envelope's error.
func (r *Refused) Unwrap() []error {
	out := make([]error, len(r.Envelopes))
	for i, e := range r.Envelopes {
		out[i] = e.Err
	}
	return out
}

// prepareAll prepares every envelope, and refuses the batch with every envelope that
// failed rather than the first, so whoever sent it can mend them all before sending
// it again.
func (g *Gate) prepareAll(envs []facet.Envelope, bindings []facet.Binding) ([]prepared, error) {
	return g.prepareEach(envs, func(i int) (prepared, error) { return g.prepare(envs[i], bindings[i]) })
}

// prepareEach prepares every envelope through prep, refusing the batch with every
// failure at once and with any two envelopes that are one facet.
func (g *Gate) prepareEach(envs []facet.Envelope, prep func(i int) (prepared, error)) ([]prepared, error) {
	var refused Refused
	ps := make([]prepared, len(envs))
	for i, env := range envs {
		p, err := prep(i)
		if err != nil {
			refused.Envelopes = append(refused.Envelopes, RefusedEnvelope{Index: i, Type: env.Type, Err: err})
			continue
		}
		ps[i] = p
	}
	if len(refused.Envelopes) > 0 {
		return nil, &refused
	}
	// Two envelopes for one facet cannot share a transaction, and the second is
	// almost always a duplicated source row rather than an intended supersession.
	seen := make(map[string]int, len(ps))
	for i, p := range ps {
		if first, dup := seen[p.facetID]; dup {
			refused.Envelopes = append(refused.Envelopes, RefusedEnvelope{Index: i, Type: p.env.Type,
				Err: fmt.Errorf("%w: envelopes %d (%s) and %d (%s) are the same facet %s, and a batch cannot hold both",
					ErrSameFacet, first, ps[first].env.SourceRef, i, p.env.SourceRef, p.facetID)})
			continue
		}
		seen[p.facetID] = i
	}
	if len(refused.Envelopes) > 0 {
		return nil, &refused
	}
	return ps, nil
}
