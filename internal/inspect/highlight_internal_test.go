package inspect

import (
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

func TestHighlightJSON_ClassesAndEscapes(t *testing.T) {
	t.Parallel()
	src := `{"a":"<script>x</script>","b":[1,-2.5e3,true,null],"c\"d":"e"}`
	toks := highlightJSON([]byte(src))
	var text strings.Builder
	kinds := map[string]string{}
	for _, tk := range toks {
		text.WriteString(tk.Text)
		if tk.Class != "" {
			kinds[tk.Text] = tk.Class
		}
	}
	for tok, want := range map[string]string{
		`"a"`: "key", `"<script>x</script>"`: "str", `-2.5e3`: "num", `true`: "lit", `null`: "lit",
		`"c\"d"`: "key", `"e"`: "str", `{`: "pun",
	} {
		if kinds[tok] != want {
			t.Errorf("%s classed %q, want %q", tok, kinds[tok], want)
		}
	}
	// The tokens are the indented text, nothing lost and nothing added.
	if got := strings.Join(strings.Fields(text.String()), ""); got != src {
		t.Errorf("tokens rejoin to %s, want %s", got, src)
	}
	if raw := highlightJSON([]byte(`not json <b>`)); len(raw) != 1 || raw[0].Class != "" || raw[0].Text != "not json <b>" {
		t.Errorf("non-JSON = %+v, want one plain token as it is", raw)
	}
}

// The walk reads a field's description and examples, names array items with [], and
// marks the rows the meta points at.
func TestFieldsOf_AnnotationsAndRoles(t *testing.T) {
	t.Parallel()
	body := []byte(`{
	  "meta": {"facet_type": "test.thing", "definition_version": 1, "classification": "Normal",
	    "validation_mode": "strict", "source_schema": "t/1",
	    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02", "zone": "UTC"}},
	    "identity_key": ["/id"]},
	  "schema": {"type": "object", "required": ["id", "at"], "additionalProperties": false,
	    "properties": {
	      "id": {"type": "string", "minLength": 1, "description": "the row's own id", "examples": ["r-1"]},
	      "at": {"type": "string", "minLength": 1},
	      "sets": {"type": "array", "items": {"type": "object", "additionalProperties": false,
	        "properties": {"reps": {"type": "integer", "minimum": 1}}}},
	      "metrics": {"type": "object", "additionalProperties": {"type": "object", "additionalProperties": false,
	        "properties": {"avg": {"type": "number", "description": "the mean"}}}}}}}`)
	d, err := def.Parse(body)
	if err != nil {
		t.Fatalf("def.Parse: %v", err)
	}
	rows, err := fieldsOf(body, d)
	if err != nil {
		t.Fatalf("fieldsOf: %v", err)
	}
	byPtr := map[string]fieldRow{}
	for _, r := range rows {
		byPtr[r.Pointer] = r
	}
	if r := byPtr["/id"]; r.Description != "the row's own id" || len(r.Examples) != 1 || r.Examples[0] != `"r-1"` ||
		!r.Required || len(r.Roles) != 1 || r.Roles[0] != "identity" {
		t.Errorf("/id row = %+v", r)
	}
	if r := byPtr["/at"]; len(r.Roles) != 1 || r.Roles[0] != "fact time" {
		t.Errorf("/at row = %+v", r)
	}
	if r, ok := byPtr["/sets[]/reps"]; !ok || r.Depth != 1 || r.Type != "integer" || len(r.Notes) != 1 {
		t.Errorf("/sets[]/reps row = %+v (present %v)", r, ok)
	}
	if r, ok := byPtr["/metrics/*/avg"]; !ok || r.Description != "the mean" || r.Depth != 1 {
		t.Errorf("/metrics/*/avg row = %+v (present %v)", r, ok)
	}
	if byPtr["/metrics"].Type != "map of object" {
		t.Errorf("/metrics type = %q", byPtr["/metrics"].Type)
	}
	if byPtr["/sets"].Type != "array of object" {
		t.Errorf("/sets type = %q", byPtr["/sets"].Type)
	}
	if n := described(rows); n != 2 {
		t.Errorf("described = %d, want 2 (/id and /metrics/*/avg)", n)
	}
}

func TestChangedFields(t *testing.T) {
	t.Parallel()
	got := changedFields(
		[]byte(`{"a": "1", "n": {"x": 1, "y": 2}, "list": [1, 2], "gone": true}`),
		[]byte(`{"a": "1", "n": {"x": 1, "y": 3}, "list": [1, 3], "new": "z"}`))
	want := []changeRow{
		{Pointer: "/gone", Prior: "true"},
		{Pointer: "/list", Prior: "[1,2]", Value: "[1,3]"},
		{Pointer: "/n/y", Prior: "2", Value: "3"},
		{Pointer: "/new", Value: `"z"`},
	}
	if len(got) != len(want) {
		t.Fatalf("changedFields = %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if v, ok := atPointer(map[string]any{"a/b": map[string]any{"c": 1.0}}, "/a~1b/c"); !ok || v != 1.0 {
		t.Errorf("atPointer escaped = %v %v", v, ok)
	}
}
