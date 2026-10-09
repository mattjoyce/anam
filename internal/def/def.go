// Package def implements Facet Definitions: the independently-versioned, per-type
// contract the admission gate enforces (ARCHITECTURE.md §3a; journal:
// facet-definition-format).
//
// A Definition is a JSON manifest in two halves. `meta` is Anam's governance DSL —
// identity, time semantics, classification, the validation bar. `schema` is standard
// JSON Schema (draft 2020-12), closed: it declares every field a payload may hold
// (schema.go). The payload is inspected once, at the gate, then held verbatim and
// opaque; validating reads it and never rewrites it.
//
// Fields in `meta` are addressed by JSON Pointer (RFC 6901), because Anam's payloads
// are nested where the reference implementation's were flat.
package def

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/mattjoyce/anam/internal/facet"
)

// ErrBadManifest is returned by Parse for a manifest that cannot form a Definition.
var ErrBadManifest = errors.New("def: bad manifest")

// ValidationMode is the pressure valve: one mechanism serving tidy and messy sources.
type ValidationMode string

// The modes. Strict holds the payload to the schema, which is closed: a field it does
// not declare is refused. Verbatim has no schema and checks only that identity and
// time resolve. There is no mode between them; a source that grows a field gets the
// field declared, and a source with a second shape gets a second Definition.
const (
	ModeStrict   ValidationMode = "strict"
	ModeVerbatim ValidationMode = "verbatim"
)

// TimeField names where a clock lives in the payload, how to parse it, and the zone
// a zoneless value is read in. Zone is an IANA name ("Australia/Sydney", "UTC") and
// is required: a source's export writes the clock on the wall where the person stood,
// and reading it as UTC puts every fact hours from where it happened. A layout that
// carries its own offset is unaffected by the zone.
type TimeField struct {
	Pointer string `json:"pointer"`
	Layout  string `json:"layout"`
	Zone    string `json:"zone"`
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

	// PayloadKind says whether facets of this type hold their source fragment or a
	// native reference to a binary that stays in its source (§3b). Verbatim unless
	// the manifest says reference.
	PayloadKind facet.PayloadKind

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

	// schema is the compiled schema half; nil in verbatim mode.
	schema *jsonschema.Schema

	// loc is FactTime.Zone loaded, once, at Parse.
	loc *time.Location
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
		PayloadKind       string `json:"payload_kind"`
		TimeSemantics     struct {
			FactTime TimeField `json:"fact_time"`
		} `json:"time_semantics"`
		IdentityKey         []string `json:"identity_key"`
		MinBindingAssurance string   `json:"min_binding_assurance"`
		Series              *Series  `json:"series"`
	} `json:"meta"`
	Schema json.RawMessage `json:"schema"`
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
		PayloadKind:    facet.PayloadKind(m.Meta.PayloadKind),
		FactTime:       m.Meta.TimeSemantics.FactTime,
		IdentityKey:    m.Meta.IdentityKey,
		MinAssurance:   facet.BindingAssurance(m.Meta.MinBindingAssurance),
		Series:         m.Meta.Series,
	}

	// Defaults, applied before validation so the rules below see the final shape.
	if d.SchemaVersion == 0 {
		d.SchemaVersion = 1
	}
	if d.MediaType == "" {
		d.MediaType = "application/json"
	}
	if d.PayloadKind == "" {
		d.PayloadKind = facet.PayloadVerbatim
	}
	if m.Meta.MinBindingAssurance == "" {
		d.MinAssurance = facet.AssuranceMedium
	}
	if err := d.validate(); err != nil {
		return Definition{}, err
	}
	switch {
	case d.Mode == ModeVerbatim && len(m.Schema) > 0:
		return Definition{}, fmt.Errorf("%w: verbatim mode checks no schema, so it must not declare one", ErrBadManifest)
	case d.Mode == ModeStrict:
		s, err := compileSchema(string(d.Type), m.Schema, d.keyedFields())
		if err != nil {
			return Definition{}, err
		}
		d.schema = s
	}
	loc, err := time.LoadLocation(d.FactTime.Zone)
	if err != nil {
		return Definition{}, fmt.Errorf("%w: time_semantics.fact_time zone %q: %v", ErrBadManifest, d.FactTime.Zone, err)
	}
	d.loc = loc
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
	case d.Mode != ModeStrict && d.Mode != ModeVerbatim:
		return fmt.Errorf("%w: validation_mode %q", ErrBadManifest, d.Mode)
	case d.SourceSchema == "":
		return fmt.Errorf("%w: source_schema is required", ErrBadManifest)
	case !d.PayloadKind.Valid():
		return fmt.Errorf("%w: payload_kind %q", ErrBadManifest, d.PayloadKind)
	case d.FactTime.Pointer == "" || d.FactTime.Layout == "" || d.FactTime.Zone == "":
		return fmt.Errorf("%w: time_semantics.fact_time needs pointer, layout and zone", ErrBadManifest)
	case len(d.IdentityKey) == 0:
		return fmt.Errorf("%w: identity_key must declare at least one pointer", ErrBadManifest)
	case !d.MinAssurance.Valid():
		return fmt.Errorf("%w: min_binding_assurance %q", ErrBadManifest, d.MinAssurance)
	}
	for _, p := range append([]string{d.FactTime.Pointer}, d.IdentityKey...) {
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

// keyedFields is the top-level field behind every pointer that keys or places a facet:
// the fact time, the identity key and the series key. The schema must require them.
func (d Definition) keyedFields() []string {
	ptrs := append([]string{d.FactTime.Pointer}, d.IdentityKey...)
	if d.Series != nil {
		ptrs = append(ptrs, d.Series.Key...)
	}
	out := make([]string, 0, len(ptrs))
	for _, p := range ptrs {
		out = append(out, topToken(p))
	}
	return out
}
