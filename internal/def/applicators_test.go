package def_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

// applicatorManifest wraps a schema body in the smallest manifest that loads.
func applicatorManifest(schema string) string {
	return fmt.Sprintf(`{
	  "meta": {"facet_type": "test.applicator", "definition_version": 1, "classification": "Normal",
	    "validation_mode": "strict", "source_schema": "t/1",
	    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02", "zone": "UTC"}},
	    "identity_key": ["/at"]},
	  "schema": %s
	}`, schema)
}

// A branch of if, then, else, allOf, anyOf, oneOf or not narrows the object it sits in
// and closes nothing, so it needs no additionalProperties; the objects that do close a
// payload (the top level and any object under properties) still do (card 233).
func TestParse_ApplicatorBranchesNeedNoClosing(t *testing.T) {
	t.Parallel()

	const head = `"type": "object", "required": ["at", "kind"], %s "properties": {
	      "at": {"type": "string", "minLength": 1},
	      "kind": {"type": "string", "enum": ["a", "b"]},
	      "extra": %s
	    }`
	closedTop := `"additionalProperties": false,`
	plainExtra := `{"type": "string"}`
	branch := `{"properties": {"kind": {"const": "a"}}}`

	tests := []struct {
		name    string
		schema  string
		wantErr string // empty: the manifest loads
	}{
		{"then branch without additionalProperties",
			fmt.Sprintf("{"+head+`, "if": %s, "then": {"required": ["extra"], "properties": {"extra": {"type": "string"}}}}`, closedTop, plainExtra, branch), ""},
		{"if, then and else branches",
			fmt.Sprintf("{"+head+`, "if": %s, "then": %s, "else": %s}`, closedTop, plainExtra, branch, branch, branch), ""},
		{"allOf anyOf oneOf and not branches",
			fmt.Sprintf("{"+head+`, "allOf": [%s], "anyOf": [%s], "oneOf": [%s], "not": %s}`, closedTop, plainExtra, branch, branch, branch, branch), ""},
		{"a branch inside a branch",
			fmt.Sprintf("{"+head+`, "allOf": [{"if": %s, "then": %s}]}`, closedTop, plainExtra, branch, branch), ""},
		{"top level without additionalProperties",
			fmt.Sprintf("{"+head+`, "if": %s, "then": %s}`, "", plainExtra, branch, branch), "schema: an object must set additionalProperties"},
		{"top level open beside a branch",
			fmt.Sprintf("{"+head+`, "then": %s}`, `"additionalProperties": true,`, plainExtra, branch), "additionalProperties is true"},
		{"nested object property without additionalProperties",
			fmt.Sprintf("{"+head+"}", closedTop, `{"type": "object", "properties": {"x": {"type": "string"}}}`), "schema/properties/extra"},
		{"nested object under a branch without additionalProperties",
			fmt.Sprintf("{"+head+`, "then": {"properties": {"extra": {"type": "object", "properties": {"x": {"type": "string"}}}}}}`, closedTop, plainExtra), "schema/then/properties/extra"},
		{"a branch may not leave the manifest",
			fmt.Sprintf("{"+head+`, "then": {"$ref": "https://example.com/s.json"}}`, closedTop, plainExtra), "leaves the manifest"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := def.Parse([]byte(applicatorManifest(tc.schema)))
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("refused: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("admitted; want an error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

// health.intake carries an additionalProperties placeholder on each of its branches;
// the lint no longer asks for them, but the manifest still loads as it is.
func TestBundled_HealthIntakeStillLoads(t *testing.T) {
	t.Parallel()

	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	if _, ok := reg.Lookup("health.intake"); !ok {
		t.Fatal("no bundled definition for health.intake")
	}
}
