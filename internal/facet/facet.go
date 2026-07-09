// Package facet defines Anam's atomic unit: a captured source fragment, held
// verbatim, together with the metadata that governs it (ARCHITECTURE.md §3).
//
// Two invariants from the do-not-break list (§12) are structural here rather than
// merely documented:
//
//   - A Facet carries no current status. Only the immutable BirthStatus it was born
//     with is stored; current Status is derived by the fold over record-events (§9).
//     There is no place to overwrite, so the ledger cannot drift from its events.
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

// BirthStatus is the immutable state a Facet was born in. It is not the Facet's
// current state and must never be used as such — see Status.
type BirthStatus string

// The states a Facet may be born in: captured world-facts are admitted; agent findings
// are proposed (§11).
const (
	BornAdmitted BirthStatus = "admitted"
	BornProposed BirthStatus = "proposed"
)

// Valid reports whether b is a known birth status.
func (b BirthStatus) Valid() bool {
	switch b {
	case BornAdmitted, BornProposed:
		return true
	}
	return false
}

// Status is a Facet's current state, derived by the fold from its record-events.
//
// Status is deliberately not a field on Facet. Storing it would create a second source
// of truth alongside the record-events, and a place to overwrite — the two things the
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

// RecordEventKind names the way Anam's knowledge of a Facet changed.
type RecordEventKind string

// The record-event kinds. All but EventPublished advance a Facet's derived status;
// EventPublished records that a Facet first appeared in a served account and leaves
// status untouched.
const (
	EventAdmitted   RecordEventKind = "admitted"
	EventSuperseded RecordEventKind = "superseded"
	EventTombstoned RecordEventKind = "tombstoned"
	EventEndorsed   RecordEventKind = "endorsed"
	EventRejected   RecordEventKind = "rejected"
	EventPublished  RecordEventKind = "published"
)

// Valid reports whether k is a known record-event kind.
func (k RecordEventKind) Valid() bool {
	switch k {
	case EventAdmitted, EventSuperseded, EventTombstoned, EventEndorsed, EventRejected, EventPublished:
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

// Provenance is a Facet's recorded origin.
type Provenance struct {
	// Source is the capture origin, lower-case hyphenated: health-db, bp-csv.
	Source string

	// Injector is the identity that submitted the envelope, from the gate's
	// authenticated pillar.
	Injector string

	// SourceRef is the deterministic natural key of the source row, from which the
	// Facet's ID derives (§6a).
	SourceRef string

	// Method and Model are set only for derived facets: how the finding was computed.
	Method string
	Model  string

	// Watermark is set only for derived facets: the canonical record-time the finding
	// was computed from (§11).
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

	// EventTime places the Facet on the timeline; RecordedTime is when a person or
	// system documented it; IngestedTime is when Anam received it (§5).
	EventTime    time.Time
	RecordedTime time.Time
	IngestedTime time.Time

	Binding    Binding
	Provenance Provenance

	// BirthStatus is the state this artifact was born in, and is immutable. The
	// Facet's current status is derived, not stored — there is deliberately no Status
	// field here.
	BirthStatus BirthStatus

	Descriptor Descriptor

	// Payload is the source fragment held verbatim, in canonical bytes (see
	// Canonicalize). It is opaque: Anam governs on Descriptor, never on this.
	Payload []byte
}

// RecordEvent asserts that Anam's knowledge of a Facet changed, at record-time (§5).
//
// The dependency runs one way: a RecordEvent references a Facet, and a Facet never
// references a RecordEvent — the world does not know Anam exists. That asymmetry is
// what makes rebuild-from-source sound.
type RecordEvent struct {
	Kind       RecordEventKind
	FacetID    string
	RecordTime time.Time

	// Seq monotonically orders events sharing a record-time.
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
	Type      Type
	Source    string
	Injector  string
	SourceRef string

	// Payload is the raw source fragment as JSON, canonicalised on admission.
	Payload []byte

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
	case e.SourceRef == "":
		return fmt.Errorf("%w: source_ref is required", ErrInvalidEnvelope)
	case len(e.Payload) == 0:
		return fmt.Errorf("%w: payload is required", ErrInvalidEnvelope)
	}
	return nil
}
