package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// A facet page reads its payload through the Definition it was admitted under: one
// row per field, the value beside what the field means (card 71, slice 3). The words
// come from that version, not the current one, because they are what the facet was
// read under.

// payloadRow is one field of a facet's payload: the Definition's row and the value
// the payload holds there. Value is the JSON of the value; Present is false for a
// field the payload leaves out, which is absent, not zero.
type payloadRow struct {
	fieldRow
	Value   string
	Present bool
}

// payloadRows lays a facet's payload over the field rows of the Definition it was
// admitted under. Rows inside an array's items or a map's values have no single
// value, so they show the words only. When the rows cannot be built, the note says
// why, in place of the table, so a Definition that fails to parse is seen rather than
// silently losing the fields (card 100); the raw payload is still on the page.
func (s *Server) payloadRows(ctx context.Context, f facet.Facet) (rows []payloadRow, note string) {
	bodies, _, err := s.held(ctx)
	if err != nil {
		return nil, "the Definitions held could not be read: " + err.Error()
	}
	body, ok := bodies[f.Type][f.DefinitionVersion]
	if !ok {
		return nil, fmt.Sprintf("definition %s v%d, which this facet was admitted under, is not held", f.Type, f.DefinitionVersion)
	}
	d, err := def.Parse(body)
	if err != nil {
		return nil, fmt.Sprintf("definition %s v%d does not parse: %v", f.Type, f.DefinitionVersion, err)
	}
	fields, err := fieldsOf(body, d)
	if err != nil {
		return nil, fmt.Sprintf("definition %s v%d's fields could not be read: %v", f.Type, f.DefinitionVersion, err)
	}
	if len(fields) == 0 {
		return nil, ""
	}
	var doc any
	if err := json.Unmarshal(f.Payload.Bytes(), &doc); err != nil {
		return nil, "the payload is not JSON: " + err.Error()
	}
	rows = make([]payloadRow, 0, len(fields))
	for _, fr := range fields {
		row := payloadRow{fieldRow: fr}
		if !strings.Contains(fr.Pointer, "[]") && !strings.Contains(fr.Pointer, "/*") {
			if v, ok := atPointer(doc, fr.Pointer); ok {
				row.Present = true
				if _, isObj := v.(map[string]any); !isObj {
					b, _ := json.Marshal(v)
					row.Value = string(b)
				}
			}
		}
		rows = append(rows, row)
	}
	return rows, ""
}

// atPointer resolves a JSON Pointer (RFC 6901) of object keys in a decoded document.
func atPointer(doc any, ptr string) (any, bool) {
	cur := doc
	for _, tok := range strings.Split(strings.TrimPrefix(ptr, "/"), "/") {
		tok = strings.NewReplacer("~1", "/", "~0", "~").Replace(tok)
		obj, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		if cur, ok = obj[tok]; !ok {
			return nil, false
		}
	}
	return cur, true
}

// changeRow is one value that differs between two artifact versions of a facet.
type changeRow struct {
	Pointer      string
	Prior, Value string // JSON; empty when the field is absent on that side
}

// changedFields lists the leaves that differ between two payloads, by pointer, in
// pointer order. An array is compared whole, as one value: its items have no identity
// of their own across versions.
func changedFields(prior, cur []byte) []changeRow {
	var a, b any
	if json.Unmarshal(prior, &a) != nil || json.Unmarshal(cur, &b) != nil {
		return nil
	}
	pa, pb := map[string]string{}, map[string]string{}
	flatten("", a, pa)
	flatten("", b, pb)
	keys := map[string]bool{}
	for k := range pa {
		keys[k] = true
	}
	for k := range pb {
		keys[k] = true
	}
	var out []changeRow
	for k := range keys {
		if pa[k] != pb[k] {
			out = append(out, changeRow{Pointer: k, Prior: pa[k], Value: pb[k]})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Pointer < out[j].Pointer })
	return out
}

func flatten(at string, v any, out map[string]string) {
	if obj, ok := v.(map[string]any); ok && len(obj) > 0 {
		for k, sub := range obj {
			flatten(at+"/"+strings.NewReplacer("~", "~0", "/", "~1").Replace(k), sub, out)
		}
		return
	}
	b, _ := json.Marshal(v)
	out[at] = string(b)
}
