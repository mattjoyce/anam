package def

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
)

// Card 194: a type split into leaves keeps a shared core, the fields every leaf
// carries with one meaning. A manifest cannot reference another file, so each leaf
// spells the core out in full, and a table names the agreement once: for each split
// type, its leaves and the pointers they share. sharedCoreDrift holds the schema at
// each shared pointer identical across the leaves, apart from description and
// examples, which may say what a field holds for that leaf's source. The deployment
// runs it over its own table and leaves.

// split is one row of the table.
type split struct {
	Type   facet.Type   `json:"type"`
	Leaves []facet.Type `json:"leaves"`
	Shared []string     `json:"shared"`
}

// schemaAt is the schema node the tree declares at a payload pointer.
func schemaAt(tree map[string]any, p string) (map[string]any, error) {
	if err := checkPointerSyntax(p); err != nil {
		return nil, err
	}
	node := tree
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
		return nil, fmt.Errorf("%w: no field %s", ErrPointer, p)
	}
	return node, nil
}

// sharedCoreDrift names the first shared pointer whose schema differs between the
// first leaf and another, prose aside, or that a leaf does not declare.
func sharedCoreDrift(s split, leaves []Definition) error {
	if len(leaves) < 2 {
		return fmt.Errorf("%s: a split names at least two leaves", s.Type)
	}
	for _, p := range s.Shared {
		var want any
		for i, d := range leaves {
			node, err := schemaAt(d.tree, p)
			if err != nil {
				return fmt.Errorf("%s: %w", d.Type, err)
			}
			got := withoutProse(node, false)
			if i == 0 {
				want = got
				continue
			}
			if !reflect.DeepEqual(got, want) {
				return fmt.Errorf("%s drifts from %s at %s", d.Type, leaves[0].Type, p)
			}
		}
	}
	return nil
}

// sessionLeaf is a leaf the test writes: a session with an id, a time interval and a
// distance, the fields a split type's leaves would share.
const sessionLeaf = `{
  "meta": {"facet_type": "test.session.a", "definition_version": 1, "classification": "Normal",
    "validation_mode": "strict", "source_schema": "test/session",
    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
    "identity_key": ["/id"]},
  "schema": {
    "type": "object", "required": ["id", "at", "effective_time_frame", "distance"], "additionalProperties": false,
    "properties": {
      "id": {"type": "string", "minLength": 1, "description": "The source's own id."},
      "at": {"type": "string", "minLength": 1},
      "effective_time_frame": {"type": "object", "required": ["time_interval"], "additionalProperties": false,
        "properties": {"time_interval": {"type": "object", "required": ["start_date_time", "end_date_time"], "additionalProperties": false,
          "properties": {"start_date_time": {"type": "string", "minLength": 1}, "end_date_time": {"type": "string", "minLength": 1}}}}},
      "distance": {"type": "object", "required": ["value", "unit"], "additionalProperties": false,
        "description": "How far, as the source reports it.",
        "properties": {"value": {"type": "number", "description": "The distance."}, "unit": {"type": "string", "enum": ["m", "km"]}}}
    }
  }
}`

// The check fails when one leaf's distance drifts from another's, and passes when
// only the words differ. Its subject is the check, so it reads a leaf the test writes,
// not a catalog's: which leaves a deployment splits, and whether they agree, is the
// deployment's to test.
func TestSharedCore_FailsOnADrift(t *testing.T) {
	t.Parallel()
	leaf := func(typ facet.Type, edit func(distance map[string]any)) Definition {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal([]byte(sessionLeaf), &m); err != nil {
			t.Fatal(err)
		}
		m["meta"].(map[string]any)["facet_type"] = string(typ)
		props := m["schema"].(map[string]any)["properties"].(map[string]any)
		edit(props["distance"].(map[string]any))
		body, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		d, err := Parse(body)
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	same := func(map[string]any) {}
	s := split{
		Type:   "test.session",
		Leaves: []facet.Type{"test.session.a", "test.session.b"},
		Shared: []string{"/id", "/effective_time_frame/time_interval", "/distance"},
	}
	cases := []struct {
		name  string
		edit  func(map[string]any)
		drift bool
	}{
		{"identical", same, false},
		{"only the words differ", func(d map[string]any) {
			d["description"] = "Metres rowed, as the Logbook reports them."
			d["examples"] = []any{map[string]any{"value": 2000, "unit": "m"}}
			d["properties"].(map[string]any)["value"].(map[string]any)["description"] = "The rower's distance."
		}, false},
		{"a value that may not be a decimal", func(d map[string]any) {
			d["properties"].(map[string]any)["value"].(map[string]any)["type"] = "integer"
		}, true},
		{"a unit no longer required", func(d map[string]any) {
			d["required"] = []any{"value"}
		}, true},
	}
	for _, c := range cases {
		leaves := []Definition{leaf(s.Leaves[0], same), leaf(s.Leaves[1], c.edit)}
		err := sharedCoreDrift(s, leaves)
		if c.drift && (err == nil || !strings.Contains(err.Error(), "at /distance")) {
			t.Errorf("%s: %v, want a drift at /distance", c.name, err)
		}
		if !c.drift && err != nil {
			t.Errorf("%s: %v, want no drift", c.name, err)
		}
	}
	if err := sharedCoreDrift(split{Type: s.Type, Shared: []string{"/nope"}}, []Definition{leaf(s.Leaves[0], same), leaf(s.Leaves[1], same)}); err == nil {
		t.Error("a shared pointer no leaf declares passed")
	}
}
