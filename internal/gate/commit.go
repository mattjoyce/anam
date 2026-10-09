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

// Commit enforces the remaining pillars and appends one envelope, returning what
// became of it and the watermark the ledger ended at. Its outcome is decided by
// identity and content (§6a): an unknown source_ref admits, a known one with the same
// hash is a no-op with no write, a known one with a different hash supersedes.
//
// It is CommitBatch of one, not a second implementation of it. It kept its own copy of
// every pillar until the two were held identical only by a test comparing their
// output; the batch path is what ingest runs, so this is the one that gives way.
func (g *Gate) Commit(ctx context.Context, env facet.Envelope, binding facet.Binding) (Result, ledger.Watermark, error) {
	results, w, err := g.CommitBatch(ctx, []facet.Envelope{env}, []facet.Binding{binding})
	if err != nil {
		return Result{}, 0, err
	}
	return results[0], w, nil
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
	series    string
}

// CommitBatch admits envelopes in one transaction, over one contiguous block of seqs.
// It is the admission path — Commit is this with a slice of one.
//
// The win is fewer commits. Every transaction ends in a sync to disk, so one envelope
// per transaction pays that sync per facet and a block of N pays it once. The head is
// read once for the block and the dispositions collapse into one negative probe.
//
// Results are returned positionally, one per input envelope, alongside the watermark
// the ledger ended at — one number for the call, because one transaction leaves the
// ledger in one place. A validation failure in any envelope fails the call and writes
// nothing: a batch that silently dropped a bad row would make "the import ran" mean
// less than it does now.
func (g *Gate) CommitBatch(ctx context.Context, envs []facet.Envelope, bindings []facet.Binding) ([]Result, ledger.Watermark, error) {
	if len(envs) != len(bindings) {
		return nil, 0, fmt.Errorf("gate: %d envelopes but %d bindings", len(envs), len(bindings))
	}
	if len(envs) == 0 {
		return nil, 0, nil
	}
	if len(envs) > MaxBatch {
		return nil, 0, fmt.Errorf("gate: batch of %d exceeds %d", len(envs), MaxBatch)
	}

	ps, err := g.prepareAll(envs, bindings)
	if err != nil {
		return nil, 0, err
	}

	results := make([]Result, len(ps))
	pending := make([]int, len(ps)) // indexes still to write, in order
	for i := range ps {
		pending[i] = i
	}

	for attempt := 0; attempt < g.maxRetries; attempt++ {
		head, err := g.ledger.Head(ctx)
		if err != nil {
			return nil, 0, err
		}

		dispositions, err := g.batchDisposition(ctx, ps, pending)
		if err != nil {
			return nil, 0, err
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
					ArtifactVersion: d.version}
				continue
			}
			f, err := g.build(ps[i], d.version)
			if err != nil {
				return nil, 0, fmt.Errorf("%s: %w", ps[i].env.SourceRef, err)
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
			// Everything resolved to a no-op, so nothing was written. This call moved
			// the watermark nowhere and head is where it ended.
			return results, head, nil
		}

		err = g.ledger.AdmitBatch(ctx, head, commits)
		var conflict *ledger.BatchConflict
		switch {
		case err == nil:
			for n, i := range writing {
				outcome := OutcomeAdmitted
				if commits[n].Event.Kind == facet.EventSuperseded {
					outcome = OutcomeSuperseded
				}
				results[i] = Result{Outcome: outcome, FacetID: ps[i].facetID,
					ArtifactVersion: commits[n].Facet.ArtifactVersion}
			}
			return results, head + ledger.Watermark(len(commits)), nil

		case errors.Is(err, ledger.ErrStaleWatermark):
			continue

		case errors.As(err, &conflict):
			if len(conflict.Divergent) > 0 {
				i := writing[conflict.Divergent[0]]
				return nil, 0, fmt.Errorf("%s: %w", ps[i].env.SourceRef, conflict.Reasons[conflict.Divergent[0]])
			}
			// Every conflict is a write that already succeeded — an earlier attempt
			// that crashed after committing. Record them and retry the remainder.
			done := map[int]bool{}
			for _, n := range conflict.AlreadyWritten {
				i := writing[n]
				done[i] = true
				results[i] = Result{Outcome: OutcomeNoOp, FacetID: ps[i].facetID,
					ArtifactVersion: commits[n].Facet.ArtifactVersion}
			}
			next := pending[:0:0]
			for _, i := range pending {
				if !done[i] {
					next = append(next, i)
				}
			}
			if len(next) == len(pending) {
				return nil, 0, fmt.Errorf("gate: batch conflict resolved nothing: %w", err)
			}
			pending = next

		default:
			return nil, 0, err
		}
	}
	return nil, 0, fmt.Errorf("%w: after %d attempts", ErrContention, g.maxRetries)
}

// prepareAll prepares every envelope, and refuses the batch with every envelope that
// failed rather than the first, so whoever sent it can mend them all before sending
// it again.
func (g *Gate) prepareAll(envs []facet.Envelope, bindings []facet.Binding) ([]prepared, error) {
	var refused Refused
	ps := make([]prepared, len(envs))
	for i, env := range envs {
		p, err := g.prepare(env, bindings[i])
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
	series, err := d.SeriesOf(doc)
	if err != nil {
		return prepared{}, err
	}
	switch env.SourceRef {
	case "":
		env.SourceRef = ref
	case ref:
	default:
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
		series: series,
	}, nil
}

// build makes the artifact at the version the disposition decided.
func (g *Gate) build(p prepared, version int) (facet.Facet, error) {
	return facet.New(facet.NewParams{
		Envelope: p.env, Binding: p.binding, Payload: p.canonical,
		ArtifactVersion: version, DefinitionVersion: p.def.Version,
		Classification: p.def.Classification,
		SourceSchema:   p.def.SourceSchema, SchemaVersion: p.def.SchemaVersion,
		MediaType: p.def.MediaType, PayloadKind: p.def.PayloadKind,
		Confidence: 1,
		FactTime:   p.factTime,
		Series:     p.series,
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
		d, err := g.disposition(ctx, ps[i].facetID, ps[i].hash)
		if err != nil {
			return nil, err
		}
		out[i] = d
	}
	return out, nil
}

// disposition is what a commit turns out to be: first sight, a re-ingest, or a change
// — and at which version. It is one value, so the function that decides it returns it
// rather than a tuple a caller has to reassemble into this.
type disposition struct {
	version int
	kind    facet.AnamEventKind
	prior   int
	noop    bool
}

// admitChecks runs the pillars that need no ledger read: structure, token scope,
// definition existence and version, assurance floor, size, and compliance.
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
	if env.DefinitionVersion != 0 && env.DefinitionVersion != d.Version {
		return def.Definition{}, nil, fmt.Errorf("%w: shaped to v%d, %s is v%d",
			ErrDefinitionVersion, env.DefinitionVersion, env.Type, d.Version)
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

// disposition decides what this commit is by asking the ledger for the facet's
// current version.
func (g *Gate) disposition(ctx context.Context, facetID, hash string) (disposition, error) {
	current, err := g.ledger.CurrentVersion(ctx, facetID)
	switch {
	case errors.Is(err, ledger.ErrNotFound):
		return disposition{version: 1, kind: facet.EventAdmitted}, nil
	case err != nil:
		return disposition{}, err
	case current.Descriptor.ContentHash == hash:
		return disposition{version: current.ArtifactVersion, noop: true}, nil
	default:
		return disposition{
			version: current.ArtifactVersion + 1,
			kind:    facet.EventSuperseded,
			prior:   current.ArtifactVersion,
		}, nil
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
func (g *Gate) Retract(ctx context.Context, facetID string) (Result, ledger.Watermark, error) {
	current, err := g.ledger.CurrentVersion(ctx, facetID)
	if err != nil {
		return Result{}, 0, err
	}

	for attempt := 0; attempt < g.maxRetries; attempt++ {
		head, err := g.ledger.Head(ctx)
		if err != nil {
			return Result{}, 0, err
		}

		events, err := g.ledger.Replay(ctx, 0, head)
		if err != nil {
			return Result{}, 0, err
		}
		states, err := fold.Fold(events)
		if err != nil {
			return Result{}, 0, err
		}
		if st := states[facetID]; st.Status == facet.StatusTombstoned {
			return Result{Outcome: OutcomeNoOp, FacetID: facetID,
				ArtifactVersion: st.Version}, head, nil
		}

		ev := facet.AnamEvent{
			Kind: facet.EventTombstoned, FacetID: facetID, AnamTime: g.clock.Now(),
			Seq: int64(head) + 1, ArtifactVersion: current.ArtifactVersion,
		}
		switch err := g.ledger.Mark(ctx, head, ev); {
		case err == nil:
			return Result{Outcome: OutcomeTombstoned, FacetID: facetID,
				ArtifactVersion: current.ArtifactVersion}, head + 1, nil
		case errors.Is(err, ledger.ErrStaleWatermark):
			continue
		default:
			return Result{}, 0, err
		}
	}
	return Result{}, 0, fmt.Errorf("%w: after %d attempts", ErrContention, g.maxRetries)
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
