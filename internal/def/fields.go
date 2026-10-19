package def

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// CheckPointer reports whether p is a well-formed JSON pointer: what a read's
// fields are named by.
func CheckPointer(p string) error { return checkPointerSyntax(p) }

// Resolve walks a decoded payload (Decode) to the pointer's target. It is the
// gate's own resolver, exported so a read's fields are cut by the same rules the
// identity key and the fact time are read by. ErrPointer when the payload lacks it.
func Resolve(doc any, pointer string) (any, error) { return resolve(doc, pointer) }

// HasPointer reports whether the schema declares a field at p: a property at each
// object token, an item at each array index. A read that names a field the type's
// schema does not declare is refused before anything is read, the way an unknown
// filter is; a type in verbatim mode declares no schema, so none of its fields can
// be named.
func (d Definition) HasPointer(p string) error {
	if err := checkPointerSyntax(p); err != nil {
		return err
	}
	if d.tree == nil {
		return fmt.Errorf("%w: %s declares no schema, so its fields cannot be named", ErrPointer, d.Type)
	}
	node := d.tree
	for _, token := range strings.Split(p[1:], "/") {
		token = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
		if props, _ := node["properties"].(map[string]any); props != nil {
			if sub, ok := props[token].(map[string]any); ok {
				node = sub
				continue
			}
		}
		if items, _ := node["items"].(map[string]any); items != nil {
			if _, err := strconv.Atoi(token); err == nil {
				node = items
				continue
			}
		}
		return fmt.Errorf("%w: %s declares no field %s (no %q at that level)", ErrPointer, d.Type, p, token)
	}
	return nil
}

// Pointers lists the fields the schema declares, as JSON pointers: each property in
// key order, descending into objects, and an array's items at index 0 (any index
// resolves). Empty for a verbatim type, which declares no schema.
func (d Definition) Pointers() []string {
	if d.tree == nil {
		return nil
	}
	return pointersOf(d.tree, "")
}

// SchemaPointers is Pointers over a manifest body at any version, read only as far
// as its schema. A history body is words, not a contract (card 111), so it is not
// held to the lint of today; what it declared is still worth knowing, because a
// facet admitted under it has the shape it describes.
func SchemaPointers(manifest []byte) ([]string, error) {
	var m struct {
		Schema map[string]any `json:"schema"`
	}
	if err := json.Unmarshal(manifest, &m); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadManifest, err)
	}
	if m.Schema == nil {
		return nil, nil
	}
	return pointersOf(m.Schema, ""), nil
}

func pointersOf(node map[string]any, prefix string) []string {
	var out []string
	if props, _ := node["properties"].(map[string]any); props != nil {
		keys := make([]string, 0, len(props))
		for k := range props {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := prefix + "/" + strings.ReplaceAll(strings.ReplaceAll(k, "~", "~0"), "/", "~1")
			out = append(out, p)
			if sub, ok := props[k].(map[string]any); ok {
				out = append(out, pointersOf(sub, p)...)
			}
		}
	}
	if items, _ := node["items"].(map[string]any); items != nil {
		p := prefix + "/0"
		out = append(out, p)
		out = append(out, pointersOf(items, p)...)
	}
	return out
}
