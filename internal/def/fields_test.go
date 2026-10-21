package def_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

const nestedManifest = `{
  "meta": {
    "facet_type": "exercise.row",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "erg/row",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "required": ["measured_at"],
    "properties": {
      "measured_at": {"type": "string", "minLength": 1},
      "distance": {"type": "object", "properties": {"value": {"type": "number"}, "unit": {"type": "string", "minLength": 1}}, "additionalProperties": false},
      "splits": {"type": "array", "items": {"type": "object", "properties": {"m": {"type": "number"}}, "additionalProperties": false}},
      "a/b": {"type": "string", "minLength": 1}
    },
    "additionalProperties": false
  }
}`

// An open object: its members are keyed by the payload, so the schema declares what
// each member holds and not what it is called.
const openManifest = `{
  "meta": {
    "facet_type": "exercise.metrics",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "erg/metrics",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "required": ["measured_at"],
    "properties": {
      "measured_at": {"type": "string", "minLength": 1},
      "metrics": {"type": "object", "additionalProperties": {"type": "number"}}
    },
    "additionalProperties": false
  }
}`

// A union type: a property may hold a container or nothing at all, or a leaf or
// nothing at all.
const unionManifest = `{
  "meta": {
    "facet_type": "exercise.union",
    "definition_version": 1,
    "classification": "Normal",
    "validation_mode": "strict",
    "source_schema": "erg/union",
    "time_semantics": {
      "fact_time": {"pointer": "/measured_at", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}
    },
    "identity_key": ["/measured_at"]
  },
  "schema": {
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    "type": "object",
    "required": ["measured_at"],
    "properties": {
      "measured_at": {"type": "string", "minLength": 1},
      "maybe_object": {"type": ["object", "null"], "properties": {"value": {"type": "number"}}, "additionalProperties": false},
      "maybe_string": {"type": ["string", "null"], "minLength": 1}
    },
    "additionalProperties": false
  }
}`

// A field is one the schema declares: a property at each object token, an item at
// each array index. Anything else is refused by name, so a read that names it fails
// before anything is read.
func TestHasPointer_WalksTheSchema(t *testing.T) {
	t.Parallel()
	d := mustParse(t, nestedManifest)
	for _, p := range []string{"/measured_at", "/distance", "/distance/value", "/splits", "/splits/0/m", "/splits/12", "/a~1b"} {
		if err := d.HasPointer(p); err != nil {
			t.Errorf("HasPointer(%s) = %v, want declared", p, err)
		}
	}
	for _, tt := range []struct{ p, want string }{
		{"/nope", `no "nope"`},
		{"/distance/nope", `no "nope"`},
		{"/distance/value/deeper", `no "deeper"`},
		{"/splits/m", `no "m"`},
		{"measured_at", "must start with '/'"},
		{"", "must start with '/'"},
	} {
		err := d.HasPointer(tt.p)
		if !errors.Is(err, def.ErrPointer) || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("HasPointer(%q) = %v, want ErrPointer saying %q", tt.p, err, tt.want)
		}
	}
}

// A container holds no value of its own: the leaves beneath it do. The schema says
// which is which, and names the leaves, so a read that asks for a container can be
// told the ones to ask for instead.
func TestContainerAt_NamesTheLeavesUnderAContainer(t *testing.T) {
	t.Parallel()
	d := mustParse(t, nestedManifest)
	for _, c := range []struct {
		pointer   string
		container bool
		leaves    []string
	}{
		{"/measured_at", false, nil},
		{"/distance", true, []string{"/distance/unit", "/distance/value"}},
		{"/distance/value", false, nil},
		{"/splits", true, []string{"/splits/0/m"}},
		{"/a~1b", false, nil},
	} {
		container, leaves, err := d.ContainerAt(c.pointer)
		if err != nil {
			t.Errorf("ContainerAt(%s) = %v", c.pointer, err)
			continue
		}
		if container != c.container {
			t.Errorf("ContainerAt(%s) container = %v, want %v", c.pointer, container, c.container)
		}
		if !slices.Equal(leaves, c.leaves) {
			t.Errorf("ContainerAt(%s) leaves = %v, want %v", c.pointer, leaves, c.leaves)
		}
	}
}

// A property that may hold a container or nothing is a container: neither the
// object nor the null is a leaf, so the pointer can never bind. A property that may
// hold a leaf is not a container: it binds for the facts that hold a leaf.
func TestContainerAt_AUnionTypeIsAContainerOnlyWhenItHoldsNoLeaf(t *testing.T) {
	t.Parallel()
	d := mustParse(t, unionManifest)
	for _, c := range []struct {
		pointer   string
		container bool
	}{
		{"/maybe_object", true},
		{"/maybe_string", false},
	} {
		container, _, err := d.ContainerAt(c.pointer)
		if err != nil {
			t.Errorf("ContainerAt(%s) = %v", c.pointer, err)
			continue
		}
		if container != c.container {
			t.Errorf("ContainerAt(%s) container = %v, want %v", c.pointer, container, c.container)
		}
	}
}

// A container whose members are open keys has leaves, but no leaf can be named:
// the names come from the payload, not from the schema.
func TestContainerAt_AnOpenObjectNamesNoLeaf(t *testing.T) {
	t.Parallel()
	d := mustParse(t, openManifest)
	container, leaves, err := d.ContainerAt("/metrics")
	if err != nil {
		t.Fatalf("ContainerAt(/metrics) = %v", err)
	}
	if !container {
		t.Error("ContainerAt(/metrics) container = false, want true")
	}
	if len(leaves) != 0 {
		t.Errorf("ContainerAt(/metrics) leaves = %v, want none", leaves)
	}
}

// A verbatim-mode type declares no schema, so none of its fields can be named.
func TestHasPointer_VerbatimHasNoFields(t *testing.T) {
	t.Parallel()
	d := mustParse(t, strings.Replace(verbatimManifest, `"strict"`, `"verbatim"`, 1))
	if err := d.HasPointer("/systolic"); !errors.Is(err, def.ErrPointer) || !strings.Contains(err.Error(), "declares no schema") {
		t.Errorf("HasPointer on a verbatim type = %v", err)
	}
}

func TestResolve_IsTheGatesResolver(t *testing.T) {
	t.Parallel()
	doc := mustDecode(t, bpPayload)
	if v, err := def.Resolve(doc, "/systolic"); err != nil || v.(interface{ String() string }).String() != "118" {
		t.Errorf("Resolve(/systolic) = %v, %v", v, err)
	}
	if _, err := def.Resolve(doc, "/nope"); !errors.Is(err, def.ErrPointer) {
		t.Errorf("Resolve(/nope) = %v, want ErrPointer", err)
	}
	if err := def.CheckPointer("nope"); !errors.Is(err, def.ErrPointer) {
		t.Errorf("CheckPointer(nope) = %v", err)
	}
}
