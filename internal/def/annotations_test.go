package def_test

import (
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

// Annotations are standard JSON Schema and carry words, not rules: a description,
// examples and a default never make a schema open or closed. The closed-schema lint
// must not read an example's values as a subschema, even an example whose own data
// looks like one (card 71).
func TestParse_AnnotationsAreNotSchema(t *testing.T) {
	t.Parallel()
	manifest := `{
	  "meta": {"facet_type": "test.annotated", "definition_version": 1, "classification": "Normal",
	    "validation_mode": "strict", "source_schema": "t/1",
	    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02", "zone": "UTC"}},
	    "identity_key": ["/id"]},
	  "schema": {
	    "$schema": "https://json-schema.org/draft/2020-12/schema",
	    "type": "object", "description": "one thing, as the source wrote it",
	    "required": ["id", "at"], "additionalProperties": false,
	    "examples": [{"id": "r-1", "at": "2026-01-02", "shape": {"type": "object", "properties": {}}}],
	    "properties": {
	      "id": {"type": "string", "minLength": 1, "description": "the row's id", "examples": ["r-1"]},
	      "at": {"type": "string", "minLength": 1, "description": "the day", "examples": ["2026-01-02"]},
	      "shape": {"type": "object", "additionalProperties": false, "description": "a nested thing",
	        "properties": {"type": {"type": "string", "minLength": 1}},
	        "default": {"type": "object"},
	        "examples": [{"type": "object"}]}
	    }
	  }
	}`
	if _, err := def.Parse([]byte(manifest)); err != nil {
		t.Fatalf("an annotated manifest was refused: %v", err)
	}
}

// A payload field may itself be called "default" or "enum": its schema is still held
// to the closed rule, because the lint skips keywords, not field names.
func TestParse_AFieldNamedLikeAKeywordIsStillChecked(t *testing.T) {
	t.Parallel()
	manifest := `{
	  "meta": {"facet_type": "test.keyword-field", "definition_version": 1, "classification": "Normal",
	    "validation_mode": "strict", "source_schema": "t/1",
	    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02", "zone": "UTC"}},
	    "identity_key": ["/at"]},
	  "schema": {"type": "object", "required": ["at"], "additionalProperties": false,
	    "properties": {
	      "at": {"type": "string", "minLength": 1},
	      "default": {"type": "object", "properties": {"x": {"type": "string"}}}
	    }}
	}`
	if _, err := def.Parse([]byte(manifest)); err == nil {
		t.Fatal("an open object under a field named default was admitted")
	}
}
