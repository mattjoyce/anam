package gate

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Commit enforces the remaining pillars and appends. Its outcome is decided by
// identity and content (§6a): an unknown source_ref admits, a known one with the same
// hash is a no-op with no write, a known one with a different hash supersedes.
func (g *Gate) Commit(ctx context.Context, env facet.Envelope, binding facet.Binding) (Result, error) {
	d, doc, err := g.admitChecks(env, binding)
	if err != nil {
		return Result{}, err
	}

	eventTime, recordedTime, err := d.Times(doc)
	if err != nil {
		return Result{}, err
	}

	// Identity is definition-owned: recompute the natural key and require the worker
	// to agree, so a drifting worker cannot silently re-key its facets.
	ref, err := d.SourceRef(env.Source, doc)
	if err != nil {
		return Result{}, err
	}
	if ref != env.SourceRef {
		return Result{}, fmt.Errorf("%w: envelope says %q, identity_key composes %q",
			ErrIdentityMismatch, env.SourceRef, ref)
	}

	canonical, err := facet.Canonicalize(env.Payload)
	if err != nil {
		return Result{}, err
	}
	hash := facet.ContentHash(canonical)
	facetID := facet.NewID(ref)

	for attempt := 0; attempt < g.maxRetries; attempt++ {
		head, err := g.ledger.Head(ctx)
		if err != nil {
			return Result{}, err
		}

		version, kind, prior, noop, err := g.disposition(ctx, facetID, hash)
		if err != nil {
			return Result{}, err
		}
		if noop {
			return Result{Outcome: OutcomeNoOp, FacetID: facetID, ArtifactVersion: version, W: head}, nil
		}

		// now is stamped inside the loop, after the head read, so that seq order and
		// record-time order agree by construction (§8).
		now := g.clock.Now()
		f, err := facet.New(facet.NewParams{
			Envelope: env, Binding: binding, Payload: canonical,
			ArtifactVersion: version, DefinitionVersion: d.Version,
			Classification: d.Classification,
			SourceSchema:   d.SourceSchema, SchemaVersion: d.SchemaVersion, MediaType: d.MediaType,
			Confidence: 1,
			EventTime:  eventTime, RecordedTime: recordedTime, IngestedTime: now,
			BirthStatus: facet.BornAdmitted,
		})
		if err != nil {
			return Result{}, err
		}
		ev := facet.RecordEvent{
			Kind: kind, FacetID: facetID, RecordTime: now,
			Seq: int64(head) + 1, ArtifactVersion: version, PriorArtifactVersion: prior,
		}

		switch err := g.ledger.Admit(ctx, head, f, ev); {
		case err == nil:
			outcome := OutcomeAdmitted
			if kind == facet.EventSuperseded {
				outcome = OutcomeSuperseded
			}
			return Result{Outcome: outcome, FacetID: facetID, ArtifactVersion: version, W: head + 1}, nil
		case errors.Is(err, ledger.ErrAlreadyWritten):
			// A crashed earlier attempt succeeded; we are reading our own write.
			return Result{Outcome: OutcomeNoOp, FacetID: facetID, ArtifactVersion: version, W: head}, nil
		case errors.Is(err, ledger.ErrStaleWatermark):
			continue
		default:
			return Result{}, err
		}
	}
	return Result{}, fmt.Errorf("%w: after %d attempts", ErrContention, g.maxRetries)
}

// admitChecks runs the pillars that need no ledger read: structure, token scope,
// definition existence, assurance floor, size, and compliance.
func (g *Gate) admitChecks(env facet.Envelope, binding facet.Binding) (def.Definition, any, error) {
	if err := env.Validate(); err != nil {
		return def.Definition{}, nil, err
	}
	if binding.PersonID != g.personID {
		return def.Definition{}, nil, fmt.Errorf("%w: binding is for %q", ErrAssuranceBelowFloor, binding.PersonID)
	}
	if !g.scopes.Allowed(env.Source, env.Type) {
		return def.Definition{}, nil, fmt.Errorf("%w: %s → %s", ErrNotAuthorized, env.Source, env.Type)
	}
	d, ok := g.defs.Lookup(env.Type)
	if !ok {
		return def.Definition{}, nil, fmt.Errorf("%w: %s", ErrNoDefinition, env.Type)
	}
	if assuranceRank(binding.Assurance) < assuranceRank(d.MinAssurance) {
		return def.Definition{}, nil, fmt.Errorf("%w: %s < %s", ErrAssuranceBelowFloor, binding.Assurance, d.MinAssurance)
	}
	if len(env.Payload) > g.maxPayloadBytes {
		return def.Definition{}, nil, fmt.Errorf("%w: %d bytes > %d", ErrPayloadTooLarge, len(env.Payload), g.maxPayloadBytes)
	}
	doc, err := def.Decode(env.Payload)
	if err != nil {
		return def.Definition{}, nil, err
	}
	if err := d.Check(doc); err != nil {
		return def.Definition{}, nil, err
	}
	return d, doc, nil
}

// disposition decides what this commit is: first sight, a re-ingest, or a change.
func (g *Gate) disposition(ctx context.Context, facetID, hash string) (version int, kind facet.RecordEventKind, prior int, noop bool, err error) {
	current, err := g.ledger.CurrentVersion(ctx, facetID)
	switch {
	case errors.Is(err, ledger.ErrNotFound):
		return 1, facet.EventAdmitted, 0, false, nil
	case err != nil:
		return 0, "", 0, false, err
	case current.Descriptor.ContentHash == hash:
		return current.ArtifactVersion, "", 0, true, nil
	default:
		return current.ArtifactVersion + 1, facet.EventSuperseded, current.ArtifactVersion, false, nil
	}
}

// Retract appends a tombstoned record-event against the facet's current version —
// a status transition, never a delete, never a new artifact (§6a).
//
// Known gap, accepted at R3: without the fold (R4) the gate cannot see that a facet
// is already tombstoned, so a repeated Retract appends a second tombstone event.
// Harmless to the fold, wasteful in the stream; R4 can refuse it cheaply.
func (g *Gate) Retract(ctx context.Context, facetID string) (Result, error) {
	current, err := g.ledger.CurrentVersion(ctx, facetID)
	if err != nil {
		return Result{}, err
	}

	for attempt := 0; attempt < g.maxRetries; attempt++ {
		head, err := g.ledger.Head(ctx)
		if err != nil {
			return Result{}, err
		}
		ev := facet.RecordEvent{
			Kind: facet.EventTombstoned, FacetID: facetID, RecordTime: g.clock.Now(),
			Seq: int64(head) + 1, ArtifactVersion: current.ArtifactVersion,
		}
		switch err := g.ledger.Mark(ctx, head, ev); {
		case err == nil:
			return Result{Outcome: OutcomeTombstoned, FacetID: facetID,
				ArtifactVersion: current.ArtifactVersion, W: head + 1}, nil
		case errors.Is(err, ledger.ErrStaleWatermark):
			continue
		default:
			return Result{}, err
		}
	}
	return Result{}, fmt.Errorf("%w: after %d attempts", ErrContention, g.maxRetries)
}

// assuranceRank orders the assurance levels: review < medium < high.
func assuranceRank(a facet.BindingAssurance) int {
	switch a {
	case facet.AssuranceReview:
		return 0
	case facet.AssuranceMedium:
		return 1
	case facet.AssuranceHigh:
		return 2
	}
	return -1
}
