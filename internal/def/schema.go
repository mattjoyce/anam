package def

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// The schema half is standard JSON Schema, draft 2020-12, so a payload can be checked
// anywhere a stock validator runs (Python's `jsonschema`, for one) and get the gate's
// answer. Validation reads the payload and never rewrites it: what passes is held
// byte for byte as the source wrote it.
//
// Anam adds rules about the schema itself, checked here at load:
//
//   - Closed. Every object in the schema says what it may hold: additionalProperties
//     is false, or a schema every extra member must meet (a map with open keys and a
//     fixed value shape). A field nobody declared is refused, not kept unexamined.
//     A source with a second shape gets a second Definition.
//   - Required means filled. A required property's schema refuses null, and a string
//     one refuses "". A measurement the source left blank is not a measurement taken.
//   - What keys and places a facet is required. The top-level fields behind the
//     identity key, the fact time and the series key must be listed in required, so
//     the schema cannot admit a payload the gate cannot key.
//   - Self-contained. No $ref leaves the manifest, and the compiler loads nothing.

const draft2020 = "https://json-schema.org/draft/2020-12/schema"

// compileSchema checks a manifest's schema against the rules above and compiles it.
func compileSchema(t string, raw json.RawMessage, keyed []string) (*jsonschema.Schema, error) {
	var tree map[string]any
	if err := json.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("%w: schema: %v", ErrBadManifest, err)
	}
	if err := lintSchema(tree, keyed); err != nil {
		return nil, fmt.Errorf("%w: schema: %v", ErrBadManifest, err)
	}

	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("%w: schema: %v", ErrBadManifest, err)
	}
	url := "urn:anam:definition:" + t
	c := jsonschema.NewCompiler()
	c.DefaultDraft(jsonschema.Draft2020)
	c.UseLoader(jsonschema.SchemeURLLoader{}) // no scheme registered: nothing is fetched
	if err := c.AddResource(url, doc); err != nil {
		return nil, fmt.Errorf("%w: schema: %v", ErrBadManifest, err)
	}
	s, err := c.Compile(url)
	if err != nil {
		return nil, fmt.Errorf("%w: schema: %v", ErrBadManifest, err)
	}
	return s, nil
}

func lintSchema(tree map[string]any, keyed []string) error {
	if v, ok := tree["$schema"]; ok && v != draft2020 {
		return fmt.Errorf("$schema is %v; Anam reads draft 2020-12 (%s)", v, draft2020)
	}
	if tree["type"] != "object" {
		return fmt.Errorf(`top level must be "type": "object"`)
	}
	props, _ := tree["properties"].(map[string]any)
	if len(props) == 0 {
		return fmt.Errorf("declares no properties")
	}
	required := map[string]bool{}
	list, _ := tree["required"].([]any)
	for _, r := range list {
		name, _ := r.(string)
		required[name] = true
		sub, ok := props[name].(map[string]any)
		if !ok {
			return fmt.Errorf("required %q is not a declared property", name)
		}
		if err := filledSchema(sub); err != nil {
			return fmt.Errorf("required %q: %v", name, err)
		}
	}
	for _, name := range keyed {
		if !required[name] {
			return fmt.Errorf("%q keys or places the facet, so it must be required", name)
		}
	}
	return closed("", tree)
}

// filledSchema says whether a property's schema refuses null and, for a string, "".
func filledSchema(sub map[string]any) error {
	types := typesOf(sub)
	if len(types) == 0 {
		return fmt.Errorf("names no type, so null would pass")
	}
	for _, t := range types {
		switch t {
		case "null":
			return fmt.Errorf("admits null")
		case "string":
			if n, ok := sub["minLength"].(float64); !ok || n < 1 {
				return fmt.Errorf(`admits "" (needs minLength 1)`)
			}
		}
	}
	return nil
}

func typesOf(sub map[string]any) []string {
	switch t := sub["type"].(type) {
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, v := range t {
			s, _ := v.(string)
			out = append(out, s)
		}
		return out
	}
	return nil
}

// closed walks every subschema and refuses an object left open or a $ref that leaves
// the manifest.
func closed(at string, node any) error {
	switch n := node.(type) {
	case map[string]any:
		if ref, ok := n["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
			return fmt.Errorf("%s: $ref %q leaves the manifest", where(at), ref)
		}
		if isObjectSchema(n) {
			switch ap := n["additionalProperties"].(type) {
			case bool:
				if ap {
					return fmt.Errorf("%s: additionalProperties is true; declare the fields", where(at))
				}
			case map[string]any:
				// a map: open keys, every value held to this schema
			default:
				return fmt.Errorf("%s: an object must set additionalProperties to false or a schema", where(at))
			}
		}
		for k, v := range n {
			if err := closed(at+"/"+k, v); err != nil {
				return err
			}
		}
	case []any:
		for i, v := range n {
			if err := closed(fmt.Sprintf("%s/%d", at, i), v); err != nil {
				return err
			}
		}
	}
	return nil
}

func isObjectSchema(n map[string]any) bool {
	for _, t := range typesOf(n) {
		if t == "object" {
			return true
		}
	}
	_, hasProps := n["properties"].(map[string]any)
	return hasProps
}

func where(at string) string {
	if at == "" {
		return "schema"
	}
	return "schema" + at
}
