package def

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
)

// Card 194: a type split into leaves keeps a shared core, the fields every leaf
// carries with one meaning. A manifest cannot reference another file, so each leaf
// spells the core out in full, and shared-core.json names the agreement once: for
// each split type, its leaves and the pointers they share. This test holds the schema
// at each shared pointer identical across the leaves, apart from description and
// examples, which may say what a field holds for that leaf's source.

// split is one row of shared-core.json.
type split struct {
	Type   facet.Type   `json:"type"`
	Leaves []facet.Type `json:"leaves"`
	Shared []string     `json:"shared"`
}

func loadSplits(t *testing.T) []split {
	t.Helper()
	raw, err := os.ReadFile("shared-core.json")
	if err != nil {
		t.Fatal(err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var tbl struct {
		Splits []split `json:"splits"`
	}
	if err := dec.Decode(&tbl); err != nil {
		t.Fatalf("shared-core.json: %v", err)
	}
	return tbl.Splits
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

func TestSharedCore_LeavesAgree(t *testing.T) {
	t.Parallel()
	reg, err := Bundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range loadSplits(t) {
		if len(s.Shared) == 0 {
			t.Errorf("%s names no shared pointers", s.Type)
		}
		var leaves []Definition
		for _, leaf := range s.Leaves {
			if !strings.HasPrefix(string(leaf), string(s.Type)+".") {
				t.Errorf("%s is not a leaf of %s", leaf, s.Type)
			}
			d, ok := reg.Lookup(leaf)
			if !ok {
				t.Errorf("%s: leaf %s is not bundled", s.Type, leaf)
				continue
			}
			leaves = append(leaves, d)
		}
		if err := sharedCoreDrift(s, leaves); err != nil {
			t.Error(err)
		}
	}
}

// The check fails when one leaf's distance drifts from another's, and passes when
// only the words differ.
func TestSharedCore_FailsOnADrift(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("manifests/activity.exercise-session.garmin.json")
	if err != nil {
		t.Fatal(err)
	}
	leaf := func(typ facet.Type, edit func(distance map[string]any)) Definition {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
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
		Type:   "activity.exercise-session",
		Leaves: []facet.Type{"activity.exercise-session.garmin", "activity.exercise-session.concept2"},
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
			d["properties"].(map[string]any)["value"].(map[string]any)["description"] = "Concept2's distance."
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
