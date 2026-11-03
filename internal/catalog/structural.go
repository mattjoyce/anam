package catalog

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/policy"
)

// structuralTypes are the types the engine's own code reads by name and by pointer
// (card 255, R4b). The engine ships no manifest for either: every catalog supplies
// them, and Load checks the pointers the consumers read. A catalog may lack one; the
// features that read it then degrade, and AbsentStructural says which.
var structuralTypes = [...]facet.Type{catchup.EdgeType, policy.DocumentType}

// AbsentStructural is the structural types the catalog holds no current Definition
// for, in a fixed order: record.edge, then record.document.
func (c *Catalog) AbsentStructural() []facet.Type {
	var out []facet.Type
	for _, t := range structuralTypes {
		if !c.definitions.Has(t) {
			out = append(out, t)
		}
	}
	return out
}

// structuralSchema is the part of a Definition's JSON Schema the structural check
// reads, decoded the way inspect's kindsOf reads record.edge's kinds.
type structuralSchema struct {
	Schema struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	} `json:"schema"`
}

// structuralProperty is one declared property: its type, its items' type, its enum.
type structuralProperty struct {
	Type  any `json:"type"`
	Items *struct {
		Type any `json:"type"`
	} `json:"items"`
	Enum *[]string `json:"enum"`
}

// checkStructural refuses a current record.edge or record.document whose declared
// shape a consumer cannot read, with def.ErrBadManifest naming the type, version and
// pointer. A pointer left undeclared passes: a consumer answers its absence itself.
// History is not checked; the consumers read the Definition in force.
//
//   - record.edge /from, /to, /about: lists of facet ids (catchup.EdgeEnds), so an
//     array whose items are strings.
//   - record.edge /kind: a closed list the link form offers (inspect's kindsOf), so a
//     non-empty enum.
//   - record.document /sha256: the library hash every reference is checked against
//     (daemon's checkReferences), so required.
func checkStructural(current []def.Stored) error {
	for _, s := range current {
		var check func(structuralSchema) (string, string, error)
		switch s.Type {
		case catchup.EdgeType:
			check = checkEdge
		case policy.DocumentType:
			check = checkDocument
		default:
			continue
		}
		var schema structuralSchema
		if err := json.Unmarshal(s.Body, &schema); err != nil {
			return fmt.Errorf("%w: %s v%d: %w", def.ErrBadManifest, s.Type, s.Version, err)
		}
		pointer, problem, err := check(schema)
		if err != nil {
			return fmt.Errorf("%w: %s v%d %s: %w", def.ErrBadManifest, s.Type, s.Version, pointer, err)
		}
		if problem != "" {
			return fmt.Errorf("%w: %s v%d %s: %s", def.ErrBadManifest, s.Type, s.Version, pointer, problem)
		}
	}
	return nil
}

// checkEdge returns the first record.edge pointer declared in a shape its consumers
// cannot read, and why.
func checkEdge(s structuralSchema) (pointer, problem string, err error) {
	for _, name := range []string{"from", "to", "about"} {
		p, ok, err := property(s, name)
		if err != nil {
			return "/" + name, "", err
		}
		if !ok {
			continue
		}
		if p.Type != "array" || p.Items == nil || p.Items.Type != "string" {
			return "/" + name, "declared other than as a list of facet ids (an array of strings)", nil
		}
	}
	p, ok, err := property(s, "kind")
	if err != nil {
		return "/kind", "", err
	}
	if ok && (p.Enum == nil || len(*p.Enum) == 0) {
		return "/kind", "declared without a non-empty enum of kinds", nil
	}
	return "", "", nil
}

// checkDocument returns the record.document pointer its consumers cannot rely on, and
// why.
func checkDocument(s structuralSchema) (pointer, problem string, err error) {
	if !slices.Contains(s.Schema.Required, "sha256") {
		return "/sha256", "not in the schema's required list, so a reference may name no bytes", nil
	}
	return "", "", nil
}

// property decodes one declared property; ok is false when it is not declared.
func property(s structuralSchema, name string) (structuralProperty, bool, error) {
	raw, ok := s.Schema.Properties[name]
	if !ok {
		return structuralProperty{}, false, nil
	}
	var p structuralProperty
	if err := json.Unmarshal(raw, &p); err != nil {
		return structuralProperty{}, false, err
	}
	return p, true, nil
}
