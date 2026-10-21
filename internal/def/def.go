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
// A third block, `handling`, is optional (card 166). It is what every writer and
// reader of the type must agree on for its facts to mean the same thing and the
// schema cannot say: a naming convention, an id's shape, what a disappearance means.
// Anam holds it as part of the version, serves it verbatim and checks only that it is
// an object. By convention a `guidance` key carries prose; the rest is the type's own.
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
//
// ZonePointer, optional, lets the fact name its own zone (card 198): when the payload
// holds a zone there (an offset +HH:MM, or an IANA name), the wall clock is read in it,
// and when it holds none, Zone applies as before. The self-report types point it at
// source_time's offset, which names one instant even in an hour the zone repeats. It
// is for a zoneless layout: a value that carries its own offset needs none. It is the
// one addition a later version may make to a type's fact time (Guard), since no fact
// admitted before carries it.
type TimeField struct {
	Pointer     string `json:"pointer"`
	Layout      string `json:"layout"`
	Zone        string `json:"zone"`
	ZonePointer string `json:"zone_pointer,omitempty"`
}

// Series says which facets of this type are repeated statements about one thing
// over time, so that a snapshot read can keep the latest per thing (SELF-LOG.md §2).
// Identity is too fine for that (every change is its own row) and type is too coarse
// (`health.medication.started` and `health.medication.stopped` are one regimen).
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

// Kind is what sort of fact a type records, declared in the manifest's meta so a
// reader can draw or filter a type it has never seen (card 176). It says what a type
// is, like Classification; it is served and read, never run by the gate. Guard and
// Register ignore it, so a version that adds or changes it keys facets as before.
type Kind string

const (
	// KindMeasure is a period's aggregate: a day of heart rate, a night of sleep.
	KindMeasure Kind = "measure"
	// KindReading is one instrument value: a blood pressure, a weight.
	KindReading Kind = "reading"
	// KindEvent is a thing that happened: an alert, a dose change, a visit.
	KindEvent Kind = "event"
)

// Valid reports whether k is one of the three kinds, or empty for a type that
// declares none.
func (k Kind) Valid() bool {
	switch k {
	case "", KindMeasure, KindReading, KindEvent:
		return true
	}
	return false
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

	// Kind is what sort of fact the type records, or "" when the manifest declares
	// none. Family is the word a reader groups the type under ("meds", "sleep"), a
	// lower-case hyphenated word, or "". Both say what the type is, for the inspector
	// and any reader; the gate runs neither (card 176).
	Kind   Kind
	Family string

	// IdentityTemplate is the type's id rule, when it declares one (card 188): a
	// string every writer builds the identity field from with ComposeID and the gate
	// checks the envelope against. "" for a type whose writers choose the id. It is
	// part of the version, and what its placeholders read must be required by the
	// schema, as the identity key is. The grammar is in idtemplate.go.
	IdentityTemplate string
	idParts          []idPart

	// Handling is the manifest's `handling` block as written, a JSON object, or nil
	// when the manifest has none. Anam reads nothing inside it: it is served to whoever
	// writes or reads facets of the type, and a change to it is a new version.
	Handling json.RawMessage

	// schema is the compiled schema half; nil in verbatim mode. tree is the same
	// schema as decoded JSON, walked to answer whether a field exists (HasPointer).
	schema *jsonschema.Schema
	tree   map[string]any

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
		Kind                string   `json:"kind"`
		Family              string   `json:"family"`
		IdentityTemplate    string   `json:"identity_template"`
	} `json:"meta"`
	Schema   json.RawMessage `json:"schema"`
	Handling json.RawMessage `json:"handling"`
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
		Kind:           Kind(m.Meta.Kind),
		Family:         m.Meta.Family,

		IdentityTemplate: m.Meta.IdentityTemplate,
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
	if d.IdentityTemplate != "" {
		parts, err := parseTemplate(d.IdentityTemplate)
		if err == nil {
			err = d.checkTemplate(parts)
		}
		if err != nil {
			return Definition{}, fmt.Errorf("%w: identity_template %q: %v", ErrBadManifest, d.IdentityTemplate, err)
		}
		d.idParts = parts
	}
	if len(m.Handling) > 0 {
		var block map[string]json.RawMessage
		if err := json.Unmarshal(m.Handling, &block); err != nil || block == nil {
			return Definition{}, fmt.Errorf("%w: handling must be a JSON object", ErrBadManifest)
		}
		d.Handling = m.Handling
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
		// compileSchema has just decoded the same bytes, so this cannot fail.
		_ = json.Unmarshal(m.Schema, &d.tree)
		if err := checkSourceTimeShape(d.tree); err != nil {
			return Definition{}, err
		}
		if err := d.declaresOptional(); err != nil {
			return Definition{}, err
		}
	case d.FactTime.ZonePointer != "" || len(optionalPointers(d.idParts)) > 0:
		return Definition{}, fmt.Errorf("%w: a verbatim type declares no fields, so it cannot name an optional one "+
			"(a zone_pointer or an optional id placeholder)", ErrBadManifest)
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
	case !d.Kind.Valid():
		return fmt.Errorf("%w: kind %q is not measure, reading or event", ErrBadManifest, d.Kind)
	case d.Family != "" && !validFamily(d.Family):
		return fmt.Errorf("%w: family %q is not a lower-case hyphenated word", ErrBadManifest, d.Family)
	}
	for _, p := range append([]string{d.FactTime.Pointer}, d.IdentityKey...) {
		if err := checkPointerSyntax(p); err != nil {
			return fmt.Errorf("%w: %v", ErrBadManifest, err)
		}
	}
	if err := d.validateZonePointers(); err != nil {
		return err
	}
	return d.validateSeries()
}

// validFamily is the shape of one segment of a type: lower-case letters and digits
// with hyphens inside, as the naming standard has every lower-case identifier.
func validFamily(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
		default:
			return false
		}
	}
	return true
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
// the fact time, the identity key, the identity template's reads and the series key. The schema must require them.
func (d Definition) keyedFields() []string {
	ptrs := append([]string{d.FactTime.Pointer}, d.IdentityKey...)
	ptrs = append(ptrs, templatePointers(d.idParts)...)
	if d.Series != nil {
		ptrs = append(ptrs, d.Series.Key...)
	}
	out := make([]string, 0, len(ptrs))
	for _, p := range ptrs {
		out = append(out, topToken(p))
	}
	return out
}
