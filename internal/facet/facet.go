// Package facet defines Anam's atomic unit: a captured source fragment, held
// verbatim, together with the metadata that governs it (ARCHITECTURE.md §3).
//
// Two invariants from the do-not-break list (§12) are structural here rather than
// merely documented:
//
//   - A Facet carries no status at all. Current Status is derived by the fold over
//     anam-events (§9). There is no place to overwrite, so the ledger cannot drift
//     from its events.
//   - A Facet's payload is opaque bytes, never a parsed map (§3). Nothing downstream
//     can normalise a source fragment into a canonical schema, because by the time it
//     is a Facet the structure is gone.
package facet

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// ErrInvalidEnvelope is returned by Envelope.Validate for a structurally malformed
// envelope. Facet-compliance against the type's Definition is a separate, later check
// performed by the admission gate.
var ErrInvalidEnvelope = errors.New("invalid envelope")

// Type is a Facet's kind: lower-case, hyphen-separated segments within dot-separated
// namespaces — health.blood-pressure, calendar.event (AGENTS.md, Naming). The
// namespace earns its keep as sources multiply.
type Type string

var typePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*(?:\.[a-z][a-z0-9]*(?:-[a-z0-9]+)*)+$`)

// Valid reports whether t is a well-formed, namespaced facet type.
func (t Type) Valid() bool { return typePattern.MatchString(string(t)) }

// Namespace returns t's leading segment ("health" for "health.sleep"), or "" if t is
// not namespaced.
func (t Type) Namespace() string {
	before, _, found := strings.Cut(string(t), ".")
	if !found {
		return ""
	}
	return before
}

// Classification is the governance level carried by a Facet and its Definition. It is
// recorded and passed through; gating on it is deferred (§10).
type Classification string

// The classification levels, in increasing order of restriction.
const (
	ClassificationNormal       Classification = "Normal"
	ClassificationConfidential Classification = "Confidential"
	ClassificationSensitive    Classification = "Sensitive"
)

// Valid reports whether c is a known classification.
func (c Classification) Valid() bool {
	switch c {
	case ClassificationNormal, ClassificationConfidential, ClassificationSensitive:
		return true
	}
	return false
}

// Status is a Facet's current state, derived by the fold from its anam-events.
//
// Status is deliberately not a field on Facet. Storing it would create a second source
// of truth alongside the anam-events, and a place to overwrite — the two things the
// append-only ledger exists to prevent.
type Status string

// The states the fold may derive for a Facet.
const (
	StatusAdmitted   Status = "admitted"
	StatusSuperseded Status = "superseded"
	StatusTombstoned Status = "tombstoned"
	StatusEndorsed   Status = "endorsed"
	StatusRejected   Status = "rejected"
)

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	switch s {
	case StatusAdmitted, StatusSuperseded, StatusTombstoned, StatusEndorsed, StatusRejected:
		return true
	}
	return false
}

// AnamEventKind names the way Anam's knowledge of a Facet changed.
type AnamEventKind string

// The anam-event kinds — exactly five, one per derivable status. Publication is
// deliberately not among them: serving a facet changes nothing Anam knows about it,
// and a receipt already records what was shown (§9). Every kind can change the
// live-set, which is what makes the watermark the anam-event sequence itself.
const (
	EventAdmitted   AnamEventKind = "admitted"
	EventSuperseded AnamEventKind = "superseded"
	EventTombstoned AnamEventKind = "tombstoned"
	EventEndorsed   AnamEventKind = "endorsed"
	EventRejected   AnamEventKind = "rejected"
)

// Valid reports whether k is a known anam-event kind.
func (k AnamEventKind) Valid() bool {
	switch k {
	case EventAdmitted, EventSuperseded, EventTombstoned, EventEndorsed, EventRejected:
		return true
	}
	return false
}

// PayloadKind says whether a Facet holds its source fragment or points at it.
type PayloadKind string

// The payload kinds. Anam's first slice is entirely verbatim; native references are
// reserved for binary sources.
const (
	PayloadVerbatim  PayloadKind = "verbatim"
	PayloadReference PayloadKind = "reference"
)

// Valid reports whether p is a known payload kind.
func (p PayloadKind) Valid() bool {
	switch p {
	case PayloadVerbatim, PayloadReference:
		return true
	}
	return false
}

// BindingAssurance attests how confidently a Facet bound to the person.
type BindingAssurance string

// The assurance levels: high when the source explicitly scopes to the person, medium
// when the binding was defaulted, review when a carried identity element contradicts.
const (
	AssuranceHigh   BindingAssurance = "high"
	AssuranceMedium BindingAssurance = "medium"
	AssuranceReview BindingAssurance = "review"
)

// Valid reports whether a is a known assurance level.
func (a BindingAssurance) Valid() bool {
	switch a {
	case AssuranceHigh, AssuranceMedium, AssuranceReview:
		return true
	}
	return false
}

// Binding is a Facet's resolution to the person, produced by the gate's resolve call.
type Binding struct {
	PersonID  string
	Assurance BindingAssurance

	// SpanHint records a span a source volunteered. It is advisory only: binding never
	// requires a span, and no span is ever a unit of consistency (§4).
	SpanHint string
}

// Provenance is a Facet's recorded origin. Capture provenance and derivation
// provenance are different concerns: every Facet has the former, only a derived Facet
// has the latter. Nesting Derivation keeps them unbraided — a captured facet with a
// model, or a derived one without a watermark, is not representable.
type Provenance struct {
	// Source is the capture origin, lower-case hyphenated: health-db, bp-csv.
	Source string

	// Injector is the identity that submitted the envelope, from the gate's
	// authenticated pillar.
	Injector string

	// SourceRef is the deterministic natural key of the source row, from which the
	// Facet's ID derives (§6a).
	SourceRef string

	// Derivation is set only on derived facets (§11); nil for captured ones.
	Derivation *Derivation
}

// Derivation records how a derived Facet was computed.
type Derivation struct {
	// Method and Model name the computation.
	Method string
	Model  string

	// Watermark is the canonical watermark the finding was computed from (§11).
	Watermark string
}

// Descriptor describes a Facet's payload without interpreting it (§3b). It is
// self-describing so that old artifacts stay readable as definitions evolve.
type Descriptor struct {
	SourceSchema  string
	SchemaVersion int
	ContentHash   string
	PayloadKind   PayloadKind
	MediaType     string
}

// Facet is Anam's atomic unit: an immutable value, never a place to update.
type Facet struct {
	// ID is deterministic in the source ref, so re-capturing a row yields the same
	// logical facet. See NewID.
	ID   string
	Type Type

	Confidence     float64
	Classification Classification

	// ArtifactVersion advances on a whole re-capture or a maturity change;
	// DefinitionVersion tracks the type contract the artifact was admitted under. The
	// two planes version independently (§3a).
	ArtifactVersion   int
	DefinitionVersion int

	// FactTime is when the fact was true in the world, read out of the payload by the
	// type's Definition (§5). It is the Facet's only clock: Anam's own clock lives on
	// the AnamEvents, which is where knowing-about belongs.
	FactTime time.Time

	// Series names the thing this facet is one statement about over time, composed by
	// the type's Definition at admission ("<family>:<key values>") so that a snapshot
	// read can group without opening a payload. Empty for a facet that stands alone.
	Series string

	Binding    Binding
	Provenance Provenance

	Descriptor Descriptor

	// Payload is the source fragment held verbatim. Its type carries the proof that it
	// is canonical, so Descriptor.ContentHash cannot have been computed over anything
	// else. It is opaque: Anam governs on Descriptor, never on this.
	Payload Canonical
}

// AnamEvent asserts that Anam's knowledge of a Facet changed, at anam-time (§5).
//
// The dependency runs one way: an AnamEvent references a Facet, and a Facet never
// references an AnamEvent — the world does not know Anam exists. That asymmetry is
// what makes rebuild-from-source sound.
type AnamEvent struct {
	Kind     AnamEventKind
	FacetID  string
	AnamTime time.Time

	// Seq monotonically orders events sharing an anam-time.
	Seq int64

	// ArtifactVersion is the Facet version this event concerns.
	ArtifactVersion int

	// PriorArtifactVersion is the version being displaced, set only for
	// EventSuperseded.
	PriorArtifactVersion int
}

// Envelope is the message a capture worker hands the admission gate. It is the only
// ingress: capture workers share nothing with the ledger except through it (§6).
type Envelope struct {
	Type     Type
	Source   string
	Injector string

	// SourceRef is optional. Left empty, the gate composes it from the Definition's
	// identity_key; carried, the gate checks it against that composition. Identity
	// is the Definition's, so a worker has no reason to compose it by hand.
	SourceRef string

	// Payload is the raw source fragment as JSON, canonicalised on admission.
	Payload []byte

	// DefinitionVersion is the Definition version the payload was shaped to, when the
	// sender says. Zero says nothing, and the gate admits against the type's current
	// version; any other value must be that version.
	DefinitionVersion int

	// AssertedPerson and SpanHint are optional; a source may volunteer either, and
	// neither is required to bind.
	AssertedPerson string
	SpanHint       string
}

// Validate checks that an Envelope is structurally well-formed. Source rows are
// external input, so this is the first of the checks that stand between them and the
// ledger; the gate's facet-compliance pillar performs the rest against the type's
// Definition.
func (e Envelope) Validate() error {
	switch {
	case !e.Type.Valid():
		return fmt.Errorf("%w: type %q is not a valid namespaced facet type", ErrInvalidEnvelope, e.Type)
	case e.Source == "":
		return fmt.Errorf("%w: source is required", ErrInvalidEnvelope)
	case e.Injector == "":
		return fmt.Errorf("%w: injector is required", ErrInvalidEnvelope)
	case len(e.Payload) == 0:
		return fmt.Errorf("%w: payload is required", ErrInvalidEnvelope)
	}
	return nil
}
