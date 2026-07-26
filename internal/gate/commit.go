package gate

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
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

	factTime, err := d.FactTimeOf(doc)
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
		// anam-time order agree by construction (§8).
		now := g.clock.Now()
		f, err := facet.New(facet.NewParams{
			Envelope: env, Binding: binding, Payload: canonical,
			ArtifactVersion: version, DefinitionVersion: d.Version,
			Classification: d.Classification,
			SourceSchema:   d.SourceSchema, SchemaVersion: d.SchemaVersion, MediaType: d.MediaType,
			Confidence: 1,
			FactTime:   factTime,
		})
		if err != nil {
			return Result{}, err
		}
		ev := facet.AnamEvent{
			Kind: kind, FacetID: facetID, AnamTime: now,
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

// prepared is one envelope past every pillar that needs no ledger read, held so a
// batch validates entirely before any of it is written.
type prepared struct {
	env       facet.Envelope
	binding   facet.Binding
	def       def.Definition
	canonical facet.Canonical
	hash      string
	facetID   string
	factTime  time.Time
}

// CommitBatch admits many envelopes in one transaction, over one contiguous block of
// seqs. It is Commit's semantics at Commit's guarantees — same pillars, same
// identity recomputation, same idempotency — differing only in how many facets share
// a round trip.
//
// The win is not fewer writes but fewer waits. Commit costs three sequential round
// trips per facet (head, disposition, transact); a batch of N costs three for the
// whole block, because the head is read once, the dispositions collapse into one
// negative probe, and the writes are one transaction.
//
// Results are returned positionally, one per input envelope. A validation failure in
// any envelope fails the call and writes nothing: a batch that silently dropped a bad
// row would make "the import ran" mean less than it does now.
func (g *Gate) CommitBatch(ctx context.Context, envs []facet.Envelope, bindings []facet.Binding) ([]Result, error) {
	if len(envs) != len(bindings) {
		return nil, fmt.Errorf("gate: %d envelopes but %d bindings", len(envs), len(bindings))
	}
	if len(envs) == 0 {
		return nil, nil
	}
	if len(envs) > ledger.MaxBatch {
		return nil, fmt.Errorf("gate: batch of %d exceeds %d", len(envs), ledger.MaxBatch)
	}

	ps := make([]prepared, len(envs))
	for i, env := range envs {
		p, err := g.prepare(env, bindings[i])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", env.SourceRef, err)
		}
		ps[i] = p
	}
	// Two envelopes for one facet cannot share a transaction, and the second is
	// almost always a duplicated source row rather than an intended supersession.
	seen := make(map[string]int, len(ps))
	for i, p := range ps {
		if first, dup := seen[p.facetID]; dup {
			return nil, fmt.Errorf("gate: envelopes %d (%s) and %d (%s) are the same facet %s — a batch cannot hold both",
				first, ps[first].env.SourceRef, i, p.env.SourceRef, p.facetID)
		}
		seen[p.facetID] = i
	}

	results := make([]Result, len(ps))
	pending := make([]int, len(ps)) // indexes still to write, in order
	for i := range ps {
		pending[i] = i
	}

	for attempt := 0; attempt < g.maxRetries; attempt++ {
		head, err := g.ledger.Head(ctx)
		if err != nil {
			return nil, err
		}

		dispositions, err := g.batchDisposition(ctx, ps, pending)
		if err != nil {
			return nil, err
		}

		// now is stamped after the head read, so seq order and anam-time order agree
		// by construction (§8). One instant for the block: the events share a
		// transaction, so they are one change in Anam's knowledge.
		now := g.clock.Now()
		commits := make([]ledger.Commit, 0, len(pending))
		writing := make([]int, 0, len(pending))
		for _, i := range pending {
			d := dispositions[i]
			if d.noop {
				results[i] = Result{Outcome: OutcomeNoOp, FacetID: ps[i].facetID,
					ArtifactVersion: d.version, W: head}
				continue
			}
			f, err := g.build(ps[i], d.version)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", ps[i].env.SourceRef, err)
			}
			commits = append(commits, ledger.Commit{
				Facet: f,
				Event: facet.AnamEvent{
					Kind: d.kind, FacetID: ps[i].facetID, AnamTime: now,
					Seq:             int64(head) + int64(len(commits)) + 1,
					ArtifactVersion: d.version, PriorArtifactVersion: d.prior,
				},
			})
			writing = append(writing, i)
		}
		if len(commits) == 0 {
			return results, nil
		}

		err = g.ledger.AdmitBatch(ctx, head, commits)
		var conflict *ledger.BatchConflict
		switch {
		case err == nil:
			w := head + ledger.Watermark(len(commits))
			for n, i := range writing {
				outcome := OutcomeAdmitted
				if commits[n].Event.Kind == facet.EventSuperseded {
					outcome = OutcomeSuperseded
				}
				results[i] = Result{Outcome: outcome, FacetID: ps[i].facetID,
					ArtifactVersion: commits[n].Facet.ArtifactVersion, W: w}
			}
			return results, nil

		case errors.Is(err, ledger.ErrStaleWatermark):
			continue

		case errors.As(err, &conflict):
			if len(conflict.Divergent) > 0 {
				i := writing[conflict.Divergent[0]]
				return nil, fmt.Errorf("%s: %w", ps[i].env.SourceRef, conflict.Reasons[conflict.Divergent[0]])
			}
			// Every conflict is a write that already succeeded — an earlier attempt
			// that crashed after committing. Record them and retry the remainder.
			done := map[int]bool{}
			for _, n := range conflict.AlreadyWritten {
				i := writing[n]
				done[i] = true
				results[i] = Result{Outcome: OutcomeNoOp, FacetID: ps[i].facetID,
					ArtifactVersion: commits[n].Facet.ArtifactVersion, W: head}
			}
			next := pending[:0:0]
			for _, i := range pending {
				if !done[i] {
					next = append(next, i)
				}
			}
			if len(next) == len(pending) {
				return nil, fmt.Errorf("gate: batch conflict resolved nothing: %w", err)
			}
			pending = next

		default:
			return nil, err
		}
	}
	return nil, fmt.Errorf("%w: after %d attempts", ErrContention, g.maxRetries)
}

// prepare runs every pillar that needs no ledger read and derives identity and
// integrity — everything Commit does before its first Head.
func (g *Gate) prepare(env facet.Envelope, binding facet.Binding) (prepared, error) {
	d, doc, err := g.admitChecks(env, binding)
	if err != nil {
		return prepared{}, err
	}
	factTime, err := d.FactTimeOf(doc)
	if err != nil {
		return prepared{}, err
	}
	ref, err := d.SourceRef(env.Source, doc)
	if err != nil {
		return prepared{}, err
	}
	if ref != env.SourceRef {
		return prepared{}, fmt.Errorf("%w: envelope says %q, identity_key composes %q",
			ErrIdentityMismatch, env.SourceRef, ref)
	}
	canonical, err := facet.Canonicalize(env.Payload)
	if err != nil {
		return prepared{}, err
	}
	return prepared{
		env: env, binding: binding, def: d, canonical: canonical,
		hash: facet.ContentHash(canonical), facetID: facet.NewID(ref), factTime: factTime,
	}, nil
}

// build makes the artifact at the version the disposition decided.
func (g *Gate) build(p prepared, version int) (facet.Facet, error) {
	return facet.New(facet.NewParams{
		Envelope: p.env, Binding: p.binding, Payload: p.canonical,
		ArtifactVersion: version, DefinitionVersion: p.def.Version,
		Classification: p.def.Classification,
		SourceSchema:   p.def.SourceSchema, SchemaVersion: p.def.SchemaVersion,
		MediaType:  p.def.MediaType,
		Confidence: 1,
		FactTime:   p.factTime,
	})
}

// batchDisposition decides first-sight / re-ingest / change for every pending
// envelope, using one negative probe instead of one query each.
//
// KnownFacets answers "unknown" with certainty and "known" only weakly, so an
// unknown facet is settled here and a known one still costs its CurrentVersion
// query. In a backfill nothing is known and no query is issued at all.
func (g *Gate) batchDisposition(ctx context.Context, ps []prepared, pending []int) (map[int]disposition, error) {
	ids := make([]string, 0, len(pending))
	for _, i := range pending {
		ids = append(ids, ps[i].facetID)
	}
	known, err := g.ledger.KnownFacets(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[int]disposition, len(pending))
	for _, i := range pending {
		if !known[ps[i].facetID] {
			out[i] = disposition{version: 1, kind: facet.EventAdmitted}
			continue
		}
		version, kind, prior, noop, err := g.disposition(ctx, ps[i].facetID, ps[i].hash)
		if err != nil {
			return nil, err
		}
		out[i] = disposition{version: version, kind: kind, prior: prior, noop: noop}
	}
	return out, nil
}

type disposition struct {
	version int
	kind    facet.AnamEventKind
	prior   int
	noop    bool
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
func (g *Gate) disposition(ctx context.Context, facetID, hash string) (version int, kind facet.AnamEventKind, prior int, noop bool, err error) {
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

// Retract appends a tombstoned anam-event against the facet's current version —
// a status transition, never a delete, never a new artifact (§6a).
//
// Retracting a facet that is already tombstoned is a no-op, decided the only way
// status is ever decided: by folding the anam-events. The check sits inside the
// retry loop, against the same head the write is conditioned on, so a raced second
// tombstone is caught on the re-read rather than appended. (Streams written by the
// R3 gate may hold a double tombstone; the fold absorbs those.)
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

		events, err := g.ledger.Replay(ctx, 0, head)
		if err != nil {
			return Result{}, err
		}
		states, err := fold.Fold(events)
		if err != nil {
			return Result{}, err
		}
		if st := states[facetID]; st.Status == facet.StatusTombstoned {
			return Result{Outcome: OutcomeNoOp, FacetID: facetID,
				ArtifactVersion: st.Version, W: head}, nil
		}

		ev := facet.AnamEvent{
			Kind: facet.EventTombstoned, FacetID: facetID, AnamTime: g.clock.Now(),
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
