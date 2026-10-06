// Package def implements Facet Definitions: the independently-versioned, per-type
// contract the admission gate enforces (ARCHITECTURE.md §3a; journal:
// facet-definition-format).
//
// A Definition is a JSON manifest in two halves. `meta` is Anam's governance DSL —
// identity, time semantics, classification, the validation bar. `schema` is a curated
// subset of JSON Schema vocabulary (required, pattern, enum, additionalProperties)
// and nothing more: the payload is inspected once, at the gate, then held verbatim
// and opaque. Running a full validator over a body Anam has promised not to
// normalise would fight the core principle (§3).
//
// Fields are addressed by JSON Pointer (RFC 6901), because Anam's payloads are
// nested where the reference implementation's were flat.
package def

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/mattjoyce/anam/internal/facet"
)

// ErrBadManifest is returned by Parse for a manifest that cannot form a Definition.
var ErrBadManifest = errors.New("def: bad manifest")

// ValidationMode is the pressure valve: one mechanism serving tidy and messy sources.
type ValidationMode string

// The modes. Strict rejects fields the schema does not name; lenient lets a source
// grow fields (the BP CSV already carries weight/SpO2/glucose sporadically); verbatim
// checks only that identity and time resolve.
const (
	ModeStrict   ValidationMode = "strict"
	ModeLenient  ValidationMode = "lenient"
	ModeVerbatim ValidationMode = "verbatim"
)

// TimeField names where a clock lives in the payload and how to parse it.
type TimeField struct {
	Pointer string `json:"pointer"`
	Layout  string `json:"layout"`
}

// Series says which facets of this type are repeated statements about one thing
// over time, so that a snapshot read can keep the latest per thing (SELF-LOG.md §2).
// Identity is too fine for that (every change is its own row) and type is too coarse
// (`med.started` and `med.stopped` are one regimen).
//
// Name is the family the series belongs to, dotted and namespaced like a facet type;
// every type that takes part in one family declares the same Name and the same Key,
// and the registry refuses a family declared two ways. Key is the ordered pointers
// whose values name one series within the family, composed by the gate as
// "<name>:<v1>[:<v2>...]" and stamped onto the facet as metadata, beside
// classification. Closes says a facet of this type ends the series it belongs to.
type Series struct {
	Name   string   `json:"name"`
	Key    []string `json:"key"`
	Closes bool     `json:"closes"`
}

// Definition is the parsed, validated contract for one facet type at one version.
type Definition struct {
	Type           facet.Type
	Version        int
	Classification facet.Classification
	Mode           ValidationMode
	SourceSchema   string
	SchemaVersion  int
	MediaType      string

	// FactTime says where in the payload the fact's own clock lives — when it was true
	// in the world. It is required, and it is the only clock a Definition declares:
	// Anam's clock is Anam's to keep, not the source's to assert.
	FactTime TimeField

	// IdentityKey is the ordered set of pointers whose values say WHICH row this is —
	// never what it contains (§6a). The gate composes source_ref from these.
	IdentityKey []string

	// MinAssurance is the association floor: review < medium < high.
	MinAssurance facet.BindingAssurance

	// Series is nil for a type whose facets stand alone. See Series.
	Series *Series

	// The schema half.
	Required             []string
	Properties           map[string]PropRule
	AdditionalProperties bool
}

// PropRule is the per-pointer constraint vocabulary: pattern and enum, nothing more.
type PropRule struct {
	Pattern *regexp.Regexp
	Enum    []string
}

// manifest mirrors the JSON layout; Parse converts and validates it.
type manifest struct {
	Meta struct {
		FacetType         string `json:"facet_type"`
		DefinitionVersion int    `json:"definition_version"`
		Classification    string `json:"classification"`
		ValidationMode    string `json:"validation_mode"`
		SourceSchema      string `json:"source_schema"`
		SchemaVersion     int    `json:"schema_version"`
		MediaType         string `json:"media_type"`
		TimeSemantics     struct {
			FactTime TimeField `json:"fact_time"`
		} `json:"time_semantics"`
		IdentityKey         []string `json:"identity_key"`
		MinBindingAssurance string   `json:"min_binding_assurance"`
		Series              *Series  `json:"series"`
	} `json:"meta"`
	Schema struct {
		Required   []string `json:"required"`
		Properties map[string]struct {
			Pattern string   `json:"pattern"`
			Enum    []string `json:"enum"`
		} `json:"properties"`
		AdditionalProperties *bool `json:"additionalProperties"`
	} `json:"schema"`
}

// Parse reads and validates one manifest. A manifest is config from a file, so
// unknown keys and structural gaps fail here, at load, not at first ingest.
func Parse(raw []byte) (Definition, error) {
	dec := json.NewDecoder(bytesReader(raw))
	dec.DisallowUnknownFields()
	var m manifest
	if err := dec.Decode(&m); err != nil {
		return Definition{}, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}

	d := Definition{
		Type:           facet.Type(m.Meta.FacetType),
		Version:        m.Meta.DefinitionVersion,
		Classification: facet.Classification(m.Meta.Classification),
		Mode:           ValidationMode(m.Meta.ValidationMode),
		SourceSchema:   m.Meta.SourceSchema,
		SchemaVersion:  m.Meta.SchemaVersion,
		MediaType:      m.Meta.MediaType,
		FactTime:       m.Meta.TimeSemantics.FactTime,
		IdentityKey:    m.Meta.IdentityKey,
		MinAssurance:   facet.BindingAssurance(m.Meta.MinBindingAssurance),
		Series:         m.Meta.Series,
		Required:       m.Schema.Required,
	}

	// Defaults, applied before validation so the rules below see the final shape.
	if d.SchemaVersion == 0 {
		d.SchemaVersion = 1
	}
	if d.MediaType == "" {
		d.MediaType = "application/json"
	}
	if m.Meta.MinBindingAssurance == "" {
		d.MinAssurance = facet.AssuranceMedium
	}
	d.AdditionalProperties = true
	if m.Schema.AdditionalProperties != nil {
		d.AdditionalProperties = *m.Schema.AdditionalProperties
	}
	if d.Mode == ModeStrict {
		d.AdditionalProperties = false
	}

	d.Properties = map[string]PropRule{}
	for ptr, rule := range m.Schema.Properties {
		pr := PropRule{Enum: rule.Enum}
		if rule.Pattern != "" {
			re, err := regexp.Compile(rule.Pattern)
			if err != nil {
				return Definition{}, fmt.Errorf("%w: property %s pattern: %v", ErrBadManifest, ptr, err)
			}
			pr.Pattern = re
		}
		d.Properties[ptr] = pr
	}

	if err := d.validate(); err != nil {
		return Definition{}, err
	}
	return d, nil
}

func (d Definition) validate() error {
	switch {
	case !d.Type.Valid():
		return fmt.Errorf("%w: facet_type %q is not a valid namespaced type", ErrBadManifest, d.Type)
	case d.Version < 1:
		return fmt.Errorf("%w: definition_version %d", ErrBadManifest, d.Version)
	case !d.Classification.Valid():
		return fmt.Errorf("%w: classification %q", ErrBadManifest, d.Classification)
	case d.Mode != ModeStrict && d.Mode != ModeLenient && d.Mode != ModeVerbatim:
		return fmt.Errorf("%w: validation_mode %q", ErrBadManifest, d.Mode)
	case d.SourceSchema == "":
		return fmt.Errorf("%w: source_schema is required", ErrBadManifest)
	case d.FactTime.Pointer == "" || d.FactTime.Layout == "":
		return fmt.Errorf("%w: time_semantics.fact_time needs pointer and layout", ErrBadManifest)
	case len(d.IdentityKey) == 0:
		return fmt.Errorf("%w: identity_key must declare at least one pointer", ErrBadManifest)
	case !d.MinAssurance.Valid():
		return fmt.Errorf("%w: min_binding_assurance %q", ErrBadManifest, d.MinAssurance)
	}
	for _, p := range append(append([]string{d.FactTime.Pointer}, d.IdentityKey...), d.Required...) {
		if err := checkPointerSyntax(p); err != nil {
			return fmt.Errorf("%w: %v", ErrBadManifest, err)
		}
	}
	for p := range d.Properties {
		if err := checkPointerSyntax(p); err != nil {
			return fmt.Errorf("%w: %v", ErrBadManifest, err)
		}
	}
	return d.validateSeries()
}

func (d Definition) validateSeries() error {
	s := d.Series
	if s == nil {
		return nil
	}
	if !facet.Type(s.Name).Valid() {
		return fmt.Errorf("%w: series.name %q is not a valid namespaced name", ErrBadManifest, s.Name)
	}
	if len(s.Key) == 0 {
		return fmt.Errorf("%w: series.key must declare at least one pointer", ErrBadManifest)
	}
	for _, p := range s.Key {
		if err := checkPointerSyntax(p); err != nil {
			return fmt.Errorf("%w: series.key: %v", ErrBadManifest, err)
		}
	}
	return nil
}
