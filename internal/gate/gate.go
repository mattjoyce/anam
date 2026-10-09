// Package gate is the admission gate: the only ingress to the ledger, enforcing the
// three pillars (ARCHITECTURE.md §6) — facet-compliant, authenticated, associated —
// and the crash-clean commit choreography (§6a, §8).
//
// Together with the append-only ledger this is Anam's error kernel. The gate owns
// what the ledger deliberately refused: the retry loop. A writer that loses the
// watermark race re-reads the head, re-stamps anam time (so seq order and
// anam-time order agree by construction), and tries again, bounded.
package gate

import (
	"context"
	"errors"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The gate's typed rejections. Every refusal names its pillar.
var (
	// ErrNotAuthorized: the authenticated pillar — this source's token is not scoped
	// for this facet type.
	ErrNotAuthorized = errors.New("gate: source not authorized for this facet type")

	// ErrNoDefinition: no contract exists for the type; nothing can be admitted
	// against a bar that is not set.
	ErrNoDefinition = errors.New("gate: no definition for facet type")

	// ErrDefinitionVersion: the envelope was shaped to a Definition version that is
	// not the type's current one.
	ErrDefinitionVersion = errors.New("gate: envelope shaped to another definition version")

	// ErrAssuranceBelowFloor: the associated pillar — the binding did not clear the
	// definition's min_binding_assurance.
	ErrAssuranceBelowFloor = errors.New("gate: binding assurance below the definition's floor")

	// ErrIdentityMismatch: the source_ref an envelope carries disagrees with the one the
	// Definition composes from the payload. Identity is definition-owned (§6a); a
	// drifting worker would silently re-key its facets.
	ErrIdentityMismatch = errors.New("gate: envelope source_ref disagrees with the definition's identity_key")

	// ErrPayloadTooLarge: the payload exceeds the gate's size policy. The ledger itself
	// would hold it; the bound is a choice about what one facet may be.
	ErrPayloadTooLarge = errors.New("gate: payload exceeds the configured maximum")

	// ErrSameFacet: two envelopes in one batch are the same facet. A batch is one
	// transaction, and one facet takes one version per transaction.
	ErrSameFacet = errors.New("gate: two envelopes in one batch are the same facet")

	// ErrContention: the bounded retry gave up. The worker may simply re-run;
	// commit is idempotent.
	ErrContention = errors.New("gate: watermark contention not resolved within retry budget")
)

// Ledger is the write-and-lookup surface the gate needs. Replay is here for one
// read: Retract folds the facet's events to refuse marking a tombstoned facet twice.
type Ledger interface {
	Head(ctx context.Context) (ledger.Watermark, error)
	CurrentVersion(ctx context.Context, facetID string) (facet.Facet, error)
	Replay(ctx context.Context, sinceW, uptoW ledger.Watermark) ([]facet.AnamEvent, error)
	FacetEvents(ctx context.Context, facetID string, uptoW ledger.Watermark) ([]facet.AnamEvent, error)
	Admit(ctx context.Context, readW ledger.Watermark, f facet.Facet, ev facet.AnamEvent) error
	Mark(ctx context.Context, readW ledger.Watermark, ev facet.AnamEvent) error

	// The batch path. KnownFacets is a fast negative used to skip per-facet
	// disposition lookups; AdmitBatch writes a block of commits over a contiguous
	// range of seqs.
	KnownFacets(ctx context.Context, facetIDs []string) (map[string]bool, error)
	AdmitBatch(ctx context.Context, readW ledger.Watermark, cs []ledger.Commit) error
}

// Definitions resolves a facet type to its contract.
type Definitions interface {
	Lookup(t facet.Type) (def.Definition, bool)
}

// TokenScopes is the authenticated pillar's policy: may this source inject this type?
type TokenScopes interface {
	Allowed(source string, t facet.Type) bool
}

// StaticScopes is the MVP's one static source → types map.
type StaticScopes map[string][]facet.Type

// Allowed reports whether the source is scoped for the type.
func (s StaticScopes) Allowed(source string, t facet.Type) bool {
	for _, allowed := range s[source] {
		if allowed == t {
			return true
		}
	}
	return false
}

// Clock is the gate's source of now — the anam-time stamped on every anam-event. It
// is the gate's alone: no source supplies it, and a Facet has nowhere to hold it.
type Clock interface {
	Now() time.Time
}

// Gate enforces admission for one person.
type Gate struct {
	ledger   Ledger
	defs     Definitions
	scopes   TokenScopes
	clock    Clock
	personID string

	maxPayloadBytes int
	maxRetries      int
}

// MaxBatch is the most envelopes one CommitBatch may carry.
//
// It is policy, and it lives here because the ledger has no limit of its own. A batch
// is one transaction, so its events share one anam-time and a stale watermark costs
// the whole block. The number is inherited: 49 was the most the ledger's previous
// substrate could write in one transaction. It is kept so that the move to SQLite
// changes where facts are stored and not which events share an instant. It can now be
// raised; doing so coarsens the anam-time resolution of an import.
const MaxBatch = 49

// New builds a Gate. maxPayloadBytes is the size policy for one payload; maxRetries
// bounds the watermark race.
func New(l Ledger, defs Definitions, scopes TokenScopes, clock Clock, personID string) *Gate {
	return &Gate{
		ledger: l, defs: defs, scopes: scopes, clock: clock, personID: personID,
		maxPayloadBytes: 300 * 1024,
		maxRetries:      5,
	}
}

// Outcome says what a commit or retract actually did.
type Outcome string

// The outcomes. NoOp is a success: the fact was already known, byte for byte.
const (
	OutcomeAdmitted   Outcome = "admitted"
	OutcomeSuperseded Outcome = "superseded"
	OutcomeNoOp       Outcome = "no-op"
	OutcomeTombstoned Outcome = "tombstoned"
)

// Result is what became of one envelope: admitted, superseded, tombstoned or a no-op,
// and at which artifact version.
//
// It carries no watermark. W is the ledger's, not an envelope's — one call leaves the
// ledger in one place — so it is returned once beside the results rather than copied
// onto each of them. Copied, the copies disagreed: a no-op is decided before the
// transaction, so it held the watermark from before the write while its written
// siblings held the one after.
type Result struct {
	Outcome         Outcome
	FacetID         string
	ArtifactVersion int
}

// Resolve is the gate's first call: the associated pillar (§6). Binding never
// requires a span; with one person the resolution is near-trivial but the shape
// holds — high when the source explicitly scopes to the person, medium when
// defaulted, review when a carried identity element contradicts.
func (g *Gate) Resolve(env facet.Envelope) (facet.Binding, error) {
	if err := env.Validate(); err != nil {
		return facet.Binding{}, err
	}
	b := facet.Binding{PersonID: g.personID, SpanHint: env.SpanHint}
	switch env.AssertedPerson {
	case g.personID:
		b.Assurance = facet.AssuranceHigh
	case "":
		b.Assurance = facet.AssuranceMedium
	default:
		b.Assurance = facet.AssuranceReview
	}
	return b, nil
}
