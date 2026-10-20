package inspect

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

// The form walk is tested on Definitions it is handed, so a schema feature the bundled
// types do not use (an enum, a whole number, a flag, a date) is drawn and read back
// too. The type below exists only here.
const formManifest = `{
  "meta": {
    "facet_type": "test.reading", "definition_version": 1, "kind": "event", "family": "tests",
    "classification": "Normal", "validation_mode": "strict", "source_schema": "test/reading",
    "time_semantics": {"fact_time": {"pointer": "/on", "layout": "2006-01-02", "zone": "Australia/Sydney"}},
    "identity_key": ["/name", "/on"], "min_binding_assurance": "medium"
  },
  "schema": {
    "type": "object", "description": "A test reading.",
    "required": ["name", "on", "level"],
    "properties": {
      "name": {"type": "string", "minLength": 1, "description": "What was read."},
      "on": {"type": "string", "minLength": 1},
      "level": {"type": "string", "enum": ["low", "high"]},
      "count": {"type": "integer", "minimum": 0},
      "fasting": {"type": "boolean"},
      "score": {"type": "number"}
    },
    "additionalProperties": false
  }
}`

func drawn(t *testing.T, lock bool) (entryForm, def.Definition) {
	t.Helper()
	d, err := def.Parse([]byte(formManifest))
	if err != nil {
		t.Fatalf("parse the fixture: %v", err)
	}
	form, err := drawForm([]byte(formManifest), d, lock)
	if err != nil {
		t.Fatalf("drawForm: %v", err)
	}
	return form, d
}

// Each property becomes the control its schema names, the fact-time field leads and
// takes a date because the layout shows no clock, and an enum is a select.
func TestDrawFormChoosesControlsFromTheSchema(t *testing.T) {
	t.Parallel()
	form, _ := drawn(t, false)
	got := map[string]string{}
	var order []string
	for _, f := range form.Fields {
		got[f.Name] = f.Input
		order = append(order, f.Name)
	}
	want := map[string]string{"on": "date", "name": "text", "level": "select", "count": "number", "fasting": "select", "score": "number"}
	for name, input := range want {
		if got[name] != input {
			t.Errorf("%s drawn as %q, want %q", name, got[name], input)
		}
	}
	if order[0] != "on" || strings.Join(order[1:3], ",") != "level,name" {
		t.Errorf("order = %v, want the fact time first, then required fields alphabetically", order)
	}
}

// What the person typed reads back as typed JSON, the date in the Definition's layout,
// and an optional field left empty is left out.
func TestReadFormTypesTheValues(t *testing.T) {
	t.Parallel()
	form, _ := drawn(t, false)
	payload, ok := form.read(url.Values{
		"f.name": {"  glucose "}, "f.on": {"2026-10-19"}, "f.level": {"high"}, "f.count": {"3"}, "f.fasting": {"true"}, "f.score": {"7.50"},
	})
	if !ok {
		t.Fatalf("read refused good values: %+v", form.Fields)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"name": "glucose", "on": "2026-10-19", "level": "high", "count": float64(3), "fasting": true, "score": 7.5}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %v (%T), want %v", k, got[k], got[k], v)
		}
	}
	payload, _ = form.read(url.Values{"f.name": {"glucose"}, "f.on": {"2026-10-19"}, "f.level": {"low"}})
	if strings.Contains(string(payload), "count") || strings.Contains(string(payload), "fasting") {
		t.Errorf("an empty optional field was sent: %s", payload)
	}
}

// A value a field does not take is a problem beside that field and no payload.
func TestReadFormNamesWhatIsNotAValue(t *testing.T) {
	t.Parallel()
	form, _ := drawn(t, false)
	_, ok := form.read(url.Values{"f.name": {"x"}, "f.on": {"not a date"}, "f.level": {"medium"}, "f.count": {"2.5"}, "f.score": {"inf"}})
	if ok {
		t.Fatal("read accepted values no field takes")
	}
	problems := map[string]bool{}
	for _, f := range form.Fields {
		problems[f.Name] = f.Problem != ""
	}
	for _, name := range []string{"on", "level", "count", "score"} {
		if !problems[name] {
			t.Errorf("%s has no problem", name)
		}
	}
	if problems["name"] {
		t.Errorf("name is fine but has a problem")
	}
}

// A correction starts from the stored values, and the fields that decide which facet
// it is lock.
func TestFillAndLockForACorrection(t *testing.T) {
	t.Parallel()
	form, _ := drawn(t, true)
	if err := form.fill([]byte(`{"name": "glucose", "on": "2026-10-19", "level": "high", "fasting": false}`)); err != nil {
		t.Fatal(err)
	}
	for _, f := range form.Fields {
		wantLocked := f.Name == "name" || f.Name == "on"
		if f.Locked != wantLocked {
			t.Errorf("%s locked = %v, want %v", f.Name, f.Locked, wantLocked)
		}
		switch f.Name {
		case "on":
			if f.Value != "2026-10-19" {
				t.Errorf("on = %q", f.Value)
			}
		case "level":
			if f.Value != "high" {
				t.Errorf("level = %q", f.Value)
			}
		case "fasting":
			if f.Value != "false" {
				t.Errorf("fasting = %q", f.Value)
			}
		}
	}
}

// The bundled types whose schemas need nested objects are refused by name, and the
// dose, which needs none, is drawn.
func TestDrawFormRefusesNestedSchemas(t *testing.T) {
	t.Parallel()
	manifests, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range manifests {
		d, ok := reg.Lookup(m.Type)
		if !ok || d.Version != m.Version || (m.Type != "health.blood-pressure" && m.Type != "health.medication.dose") {
			continue
		}
		_, err := drawForm(m.Body, d, false)
		switch m.Type {
		case "health.blood-pressure":
			if err == nil || !strings.Contains(err.Error(), "effective_time_frame") || !strings.Contains(err.Error(), "Nested objects") {
				t.Errorf("blood pressure: err = %v, want a refusal naming effective_time_frame", err)
			}
		default:
			if err != nil {
				t.Errorf("the dose is not drawn: %v", err)
			}
		}
	}
}
