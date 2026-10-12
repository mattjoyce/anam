package def_test

import (
	"errors"
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
