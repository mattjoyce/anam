package facet

import (
	"errors"
	"fmt"
	"time"
)

// ErrInvalidFacet is returned by New when the parameters cannot form a legal Facet.
var ErrInvalidFacet = errors.New("invalid facet")

// NewParams carries everything New cannot derive itself. Identity and integrity are
// deliberately absent: the ID comes from the envelope's source ref and the content
// hash from the payload, both computed inside New, so a Facet whose hash disagrees
// with its own payload — or whose ID disagrees with its source row — cannot be built.
type NewParams struct {
	Envelope Envelope
	Binding  Binding

	// Payload is the canonicalised source fragment. Only Canonicalize (or the store's
	// rehydration) can produce one, so a raw unsorted payload cannot arrive here.
	Payload Canonical

	// ArtifactVersion and DefinitionVersion: the two independent planes (§3a).
	ArtifactVersion   int
	DefinitionVersion int

	// Classification, SourceSchema, SchemaVersion and MediaType come from the type's
	// Facet Definition.
	Classification Classification
	SourceSchema   string
	SchemaVersion  int
	MediaType      string

	// Confidence is (0, 1]; captured facts are 1.
	Confidence float64

	EventTime    time.Time
	RecordedTime time.Time
	IngestedTime time.Time

	// BirthStatus and Derivation travel together: a derived facet is born proposed and
	// carries its derivation; a captured fact is born admitted and carries none (§11).
	BirthStatus BirthStatus
	Derivation  *Derivation
}

// New is the only way to build a Facet. It validates every field, derives identity
// from the source ref, and computes the content hash from the payload it was given
// (§6a: identity says which fact, the hash says whether it changed — New guarantees
// neither can be asserted independently of what they describe).
//
// The first slice is entirely verbatim, so New always sets PayloadKind accordingly;
// native references are reserved (§13) and will earn a parameter when they earn code.
func New(p NewParams) (Facet, error) {
	if err := p.validate(); err != nil {
		return Facet{}, err
	}

	return Facet{
		ID:                NewID(p.Envelope.SourceRef),
		Type:              p.Envelope.Type,
		Confidence:        p.Confidence,
		Classification:    p.Classification,
		ArtifactVersion:   p.ArtifactVersion,
		DefinitionVersion: p.DefinitionVersion,
		EventTime:         p.EventTime,
		RecordedTime:      p.RecordedTime,
		IngestedTime:      p.IngestedTime,
		Binding:           p.Binding,
		Provenance: Provenance{
			Source:     p.Envelope.Source,
			Injector:   p.Envelope.Injector,
			SourceRef:  p.Envelope.SourceRef,
			Derivation: p.Derivation,
		},
		BirthStatus: p.BirthStatus,
		Descriptor: Descriptor{
			SourceSchema:  p.SourceSchema,
			SchemaVersion: p.SchemaVersion,
			ContentHash:   ContentHash(p.Payload),
			PayloadKind:   PayloadVerbatim,
			MediaType:     p.MediaType,
		},
		Payload: p.Payload,
	}, nil
}

func (p NewParams) validate() error {
	if err := p.Envelope.Validate(); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidFacet, err)
	}
	switch {
	case p.Binding.PersonID == "":
		return fmt.Errorf("%w: binding has no person", ErrInvalidFacet)
	case !p.Binding.Assurance.Valid():
		return fmt.Errorf("%w: binding assurance %q", ErrInvalidFacet, p.Binding.Assurance)
	case p.Payload.Len() == 0:
		return fmt.Errorf("%w: empty payload", ErrInvalidFacet)
	case p.ArtifactVersion < 1:
		return fmt.Errorf("%w: artifact version %d", ErrInvalidFacet, p.ArtifactVersion)
	case p.DefinitionVersion < 1:
		return fmt.Errorf("%w: definition version %d", ErrInvalidFacet, p.DefinitionVersion)
	case !p.Classification.Valid():
		return fmt.Errorf("%w: classification %q", ErrInvalidFacet, p.Classification)
	case p.SourceSchema == "":
		return fmt.Errorf("%w: source schema is required", ErrInvalidFacet)
	case p.SchemaVersion < 1:
		return fmt.Errorf("%w: schema version %d", ErrInvalidFacet, p.SchemaVersion)
	case p.MediaType == "":
		return fmt.Errorf("%w: media type is required", ErrInvalidFacet)
	case p.Confidence <= 0 || p.Confidence > 1:
		return fmt.Errorf("%w: confidence %v is outside (0, 1]", ErrInvalidFacet, p.Confidence)
	case p.EventTime.IsZero(), p.RecordedTime.IsZero(), p.IngestedTime.IsZero():
		return fmt.Errorf("%w: all three times are required", ErrInvalidFacet)
	case !p.BirthStatus.Valid():
		return fmt.Errorf("%w: birth status %q", ErrInvalidFacet, p.BirthStatus)
	case (p.BirthStatus == BornProposed) != (p.Derivation != nil):
		return fmt.Errorf("%w: derivation and birth status disagree — derived facets are born proposed and carry their derivation; captured facts are born admitted and carry none", ErrInvalidFacet)
	}
	return nil
}
