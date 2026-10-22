package api_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// Card 211: a query's projection is a name or an inline template object on the wire,
// and a name still reads and writes as the plain string it always was.
func TestProjectionRef_IsANameOrAnInlineTemplate(t *testing.T) {
	t.Parallel()
	const inline = `{"types":["health.weight"],"statuses":["admitted"],"max_classification":"Normal"}`
	for _, tc := range []struct {
		name, in string
		want     api.ProjectionRef
	}{
		{"a name", `{"projection":"bp"}`, api.Named("bp")},
		{"an inline template", `{"projection":` + inline + `}`, api.ProjectionRef{Inline: json.RawMessage(inline)}},
		{"null", `{"projection":null}`, api.ProjectionRef{}},
		{"absent", `{}`, api.ProjectionRef{}},
	} {
		var q api.QueryRequest
		if err := json.Unmarshal([]byte(tc.in), &q); err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if q.Projection.Name != tc.want.Name || string(q.Projection.Inline) != string(tc.want.Inline) {
			t.Errorf("%s: projection = %+v, want %+v", tc.name, q.Projection, tc.want)
		}
	}

	// Anything else is refused, and says what a projection is.
	for _, in := range []string{`{"projection":7}`, `{"projection":["bp"]}`, `{"projection":true}`} {
		var q api.QueryRequest
		err := json.Unmarshal([]byte(in), &q)
		if err == nil || !strings.Contains(err.Error(), "name, or an object") {
			t.Errorf("%s: err = %v, want the refusal to say a projection is a name or an object", in, err)
		}
	}

	// Written back, a name is a string and an inline template is its object.
	for _, ref := range []api.ProjectionRef{api.Named("bp"), {Inline: json.RawMessage(inline)}} {
		b, err := json.Marshal(api.QueryRequest{Projection: ref, Text: "x", Relation: "y"})
		if err != nil {
			t.Fatal(err)
		}
		var back api.QueryRequest
		if err := json.Unmarshal(b, &back); err != nil || back.Projection.Name != ref.Name || string(back.Projection.Inline) != string(ref.Inline) {
			t.Errorf("round trip of %+v: %s, %+v, %v", ref, b, back.Projection, err)
		}
	}
	if _, err := json.Marshal(api.ProjectionRef{Name: "bp", Inline: json.RawMessage(inline)}); err == nil {
		t.Error("a projection both named and inline was written")
	}
}
