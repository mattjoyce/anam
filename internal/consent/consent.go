// Package consent is the gate a projection's every candidate facet passes through
// before it is served (ARCHITECTURE.md §10).
//
// Today it allows everything. Anam serves one person, so there is no second party to
// withhold anything from, and the policy that would decide is deferred (MVP.md §4).
// The seam exists anyway, invoked on the real path rather than reserved in a comment,
// because a gate added later has to be threaded through every caller that already
// serves, and a gate that was always there does not.
//
// What this package is not: it does not assess a facet. Anam does not read a payload,
// does not judge how sensitive a fact is, and does not decide who ought to see it. A
// facet's Classification is set by its Definition at capture and compared here as an
// ordinal; the ceiling it is compared against is a knob the projection template sets.
// Access is roles → projections, and this gate is the place a future policy would read
// the role — not a place Anam forms an opinion.
package consent

import "github.com/mattjoyce/anam/internal/facet"

// Request is what the gate is asked about: one facet's governing metadata.
//
// There is deliberately no payload field. "Filter on metadata only, never the payload"
// (§9) is then a property of the type rather than a rule the caller has to keep, and a
// future policy author cannot reach the source fragment even by accident.
type Request struct {
	Type           facet.Type
	Classification facet.Classification
	Status         facet.Status

	// FactTime is the fixed-width clock.Layout string the canonical carries, so a
	// context-of-use rule about recency has something to read.
	FactTime string
}

// Decision is the gate's answer. Reason is free text explaining a denial; it is empty
// when Allow is true, and it exists so that a later policy has somewhere to say why
// rather than silently shrinking a served value.
type Decision struct {
	Allow  bool
	Reason string
}

// Open is the default-allow gate: single-user Anam, no policy (§10).
type Open struct{}

// Evaluate allows every request.
func (Open) Evaluate(Request) Decision { return Decision{Allow: true} }
