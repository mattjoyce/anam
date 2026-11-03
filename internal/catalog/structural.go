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
// reads. Each property is kept as decoded JSON, not coerced into Go types, so a null
// enum member or a list of types is seen as written.
type structuralSchema struct {
	Schema struct {
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	} `json:"schema"`
}

// sha256Pattern is the pattern record.document's sha256 must declare: lower-case hex
// of 64 digits, the only form library.IsSHA256 and the library's file names accept.
const sha256Pattern = "^[0-9a-f]{64}$"

// checkStructural refuses a current record.edge or record.document whose declared
// shape a consumer cannot read, with def.ErrBadManifest naming the type, version and
// pointer. A pointer left undeclared passes where a consumer answers its absence
// itself. History is not checked; the consumers read the Definition in force. A
// type is read with def.TypesOf, so "string" and ["string"] are the same declaration,
// and ["string", "null"] is not.
//
//   - record.edge /from, /to, /about: lists of facet ids (catchup.EdgeEnds), so of
//     type array only, with items of type string only.
//   - record.edge /kind: one of a closed list the link form offers (inspect's
//     kindsOf), so of type string only, with an enum of non-empty strings.
//   - record.document /sha256: the library hash every reference is checked against
//     (daemon's checkReferences), so required, of type string only, with the pattern
//     of lower-case hex.
func checkStructural(current []def.Stored) error {
	for _, s := range current {
		var check func(structuralSchema) (string, string)
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
		if pointer, problem := check(schema); problem != "" {
			return fmt.Errorf("%w: %s v%d %s: %s", def.ErrBadManifest, s.Type, s.Version, pointer, problem)
		}
	}
	return nil
}

// only reports whether a subschema declares exactly one type, want.
func only(sub map[string]any, want string) bool {
	return slices.Equal(def.TypesOf(sub), []string{want})
}

// checkEdge returns the first record.edge pointer declared in a shape its consumers
// cannot read, and why.
func checkEdge(s structuralSchema) (pointer, problem string) {
	for _, name := range []string{"from", "to", "about"} {
		p, ok := s.Schema.Properties[name]
		if !ok {
			continue
		}
		items, _ := p["items"].(map[string]any)
		if !only(p, "array") || items == nil || !only(items, "string") {
			return "/" + name, "declared other than as a list of facet ids (of type array only, with items of type string only)"
		}
	}
	p, ok := s.Schema.Properties["kind"]
	if !ok {
		return "", ""
	}
	if !only(p, "string") {
		return "/kind", "declared other than of type string only"
	}
	enum, _ := p["enum"].([]any)
	if len(enum) == 0 {
		return "/kind", "declared without a non-empty enum of kinds"
	}
	for _, v := range enum {
		if k, ok := v.(string); !ok || k == "" {
			return "/kind", fmt.Sprintf("enum member %v is not a non-empty string", v)
		}
	}
	return "", ""
}

// checkDocument returns the record.document pointer its consumers cannot rely on, and
// why.
func checkDocument(s structuralSchema) (pointer, problem string) {
	if !slices.Contains(s.Schema.Required, "sha256") {
		return "/sha256", "not in the schema's required list, so a reference may name no bytes"
	}
	p := s.Schema.Properties["sha256"]
	if p == nil || !only(p, "string") || p["pattern"] != sha256Pattern {
		return "/sha256", "declared other than as a library hash (of type string only, with pattern " + sha256Pattern + ")"
	}
	return "", ""
}
