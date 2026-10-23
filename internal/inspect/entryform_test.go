package inspect

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
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

// Card 198 gave each self-report type timed by a wall clock a version whose fact time
// reads the offset the fact names, from source_time, which the form cannot draw. So the form fills it rather than
// drawing it, and every type that drew at its version before still draws, now zoned.
func TestEveryTypeThatDrewStillDrawsOnceZoned(t *testing.T) {
	t.Parallel()
	manifests, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	bodies := map[string][]byte{}
	for _, m := range manifests {
		bodies[string(m.Type)+"@"+strconv.Itoa(m.Version)] = m.Body
	}
	zoned := 0
	for _, d := range reg.All() {
		if d.FactTime.ZonePointer == "" {
			continue
		}
		zoned++
		before, ok := bodies[string(d.Type)+"@"+strconv.Itoa(d.Version-1)]
		if !ok {
			continue
		}
		old, err := def.Parse(before)
		if err != nil {
			continue // an older format than Parse reads: never drawn by this page
		}
		if _, err := drawForm(before, old, false); err != nil {
			continue // it did not draw before either
		}
		form, err := drawForm(bodies[string(d.Type)+"@"+strconv.Itoa(d.Version)], d, false)
		if err != nil {
			t.Errorf("%s v%d drew at v%d and does not now: %v", d.Type, d.Version, d.Version-1, err)
			continue
		}
		if !form.Zoned {
			t.Errorf("%s v%d's form does not send the zone", d.Type, d.Version)
		}
		for _, f := range form.Fields {
			if f.Name == def.SourceTimeField {
				t.Errorf("%s v%d draws source_time", d.Type, d.Version)
			}
		}
	}
	if zoned < 3 {
		t.Errorf("%d bundled types read a zone from the fact, want the three self-report types timed by a wall clock", zoned)
	}
}

// A zoned form sends source_time from the fields the browser fills, keeps the wall
// clock's digits as typed even where Sydney skips them, and sends nothing, saying why,
// when it has no offset.
func TestAZonedFormSendsTheBrowsersZone(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, _ := reg.Lookup("health.medication.dose")
	manifests, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	var body []byte
	for _, m := range manifests {
		if m.Type == d.Type && m.Version == d.Version {
			body = m.Body
		}
	}
	vals := url.Values{"f.drug": {"melatonin"}, "f.amount": {"5"}, "f.unit": {"mg"}}
	for _, c := range []struct {
		name, taken, zone, offset string
		want                      string
	}{
		{"London, with the offset", "2026-10-19T18:00", "Europe/London", "+01:00",
			`"source_time":{"basis":"source","offset":"+01:00","value":"2026-10-19 18:00","zone":"Europe/London"}`},
		{"an offset and no zone: the offset names it", "2026-10-19T18:00", "", "+01:00",
			`"source_time":{"basis":"source","offset":"+01:00","value":"2026-10-19 18:00","zone":"+01:00"}`},
		{"a wall clock Sydney skips keeps its digits", "2026-10-04T02:30", "Europe/London", "+01:00",
			`"taken_at":"2026-10-04 02:30"`},
	} {
		form, err := drawForm(body, d, false)
		if err != nil {
			t.Fatal(err)
		}
		v := url.Values{}
		for k, x := range vals {
			v[k] = x
		}
		v.Set("f.taken_at", c.taken)
		v.Set("zone", c.zone)
		v.Set("offset", c.offset)
		payload, ok := form.read(v)
		if !ok || !strings.Contains(string(payload), c.want) {
			t.Errorf("%s: payload %s (ok %v) lacks %s", c.name, payload, ok, c.want)
		}
	}
	form, _ := drawForm(body, d, false)
	vals.Set("f.taken_at", "2026-10-19T18:00")
	vals.Set("zone", "Europe/London")
	payload, ok := form.read(vals)
	if ok || payload != nil || !strings.Contains(form.OffsetProblem, "Anam needs the offset") {
		t.Errorf("no offset posted: payload %s, ok %v, problem %q; want nothing sent and the reason", payload, ok, form.OffsetProblem)
	}
}

// Card 222: the attention types are entered by hand, and the form is drawn from their
// Definitions with nothing about them in the page. The fact time leads and is zoned,
// the composed id is left to the gate, intensity is a whole number with its bounds
// stated, the topic and the note are text, and what the person types reads back as
// the typed JSON the Definition takes.
func TestTheAttentionTypesDrawAsForms(t *testing.T) {
	t.Parallel()
	manifests, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	drawType := func(typ string) entryForm {
		t.Helper()
		d, ok := reg.Lookup(facet.Type(typ))
		if !ok {
			t.Fatalf("%s is not bundled", typ)
		}
		for _, m := range manifests {
			if m.Type == d.Type && m.Version == d.Version {
				form, err := drawForm(m.Body, d, false)
				if err != nil {
					t.Fatalf("%s: %v", typ, err)
				}
				if !form.Zoned {
					t.Errorf("%s: the form does not send the zone", typ)
				}
				for _, f := range form.Fields {
					if f.Name == "id" || f.Name == def.SourceTimeField {
						t.Errorf("%s draws %s", typ, f.Name)
					}
				}
				return form
			}
		}
		t.Fatalf("no manifest for %s", typ)
		return entryForm{}
	}
	byName := func(form entryForm) map[string]formField {
		out := map[string]formField{}
		for _, f := range form.Fields {
			out[f.Name] = f
		}
		return out
	}

	form := drawType("mind.attention")
	if form.Fields[0].Name != "at" || form.Fields[0].Input != "datetime-local" || !form.Fields[0].FactTime {
		t.Errorf("first field = %+v, want the fact time as a date and time", form.Fields[0])
	}
	fields := byName(form)
	if f := fields["intensity"]; f.Input != "number" || f.Step != "1" || !f.Required ||
		strings.Join(f.Hints, ",") != "at least 1,at most 5" {
		t.Errorf("intensity = %+v, want a required whole number saying at least 1 and at most 5", f)
	}
	if f := fields["topic"]; f.Input != "text" || !f.Required {
		t.Errorf("topic = %+v, want required text", f)
	}
	if f := fields["note"]; f.Input != "text" || f.Required {
		t.Errorf("note = %+v, want optional text", f)
	}

	good := url.Values{"f.at": {"2026-10-22T21:40"}, "f.topic": {"deskcam"}, "f.intensity": {"4"}, "f.note": {"prints straight"},
		"zone": {"Australia/Sydney"}, "offset": {"+11:00"}}
	payload, ok := form.read(good)
	if !ok {
		t.Fatalf("read refused good values: %+v", form.Fields)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got["at"] != "2026-10-22 21:40" || got["topic"] != "deskcam" || got["intensity"] != float64(4) || got["note"] != "prints straight" {
		t.Errorf("payload = %s", payload)
	}
	if !strings.Contains(string(payload), `"intensity":4`) {
		t.Errorf("intensity was sent as text: %s", payload)
	}
	bad := url.Values{"f.at": {"2026-10-22T21:40"}, "f.topic": {"deskcam"}, "f.intensity": {"4.5"}, "zone": {"Australia/Sydney"}, "offset": {"+11:00"}}
	if _, ok := form.read(bad); ok || byName(form)["intensity"].Problem == "" {
		t.Errorf("a fractional intensity was read; problem %q", byName(form)["intensity"].Problem)
	}

	for _, typ := range []string{"mind.attention.started", "mind.attention.ended"} {
		f := byName(drawType(typ))
		if f["at"].Input != "datetime-local" || f["topic"].Input != "text" || f["note"].Required {
			t.Errorf("%s: at %+v, topic %+v, note %+v", typ, f["at"], f["topic"], f["note"])
		}
	}
}

// Card 232: health.intake is entered by hand too, and its form is drawn from the
// Definition. The kind and the part of the day are selects, a meal's two answers are
// yes or no selects, the count is a whole number from 1, and what the person types
// reads back as typed JSON. The form shows every field of every kind; the gate, not
// the page, refuses a drink with a meal's fields.
func TestTheIntakeTypeDrawsAsAForm(t *testing.T) {
	t.Parallel()
	manifests, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup("health.intake")
	if !ok {
		t.Fatal("health.intake is not bundled")
	}
	var form entryForm
	for _, m := range manifests {
		if m.Type == d.Type && m.Version == d.Version {
			if form, err = drawForm(m.Body, d, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(form.Fields) == 0 || !form.Zoned {
		t.Fatalf("no form drawn, or it does not send the zone: %+v", form)
	}
	if form.Fields[0].Name != "at" || !form.Fields[0].FactTime {
		t.Errorf("first field = %+v, want the fact time", form.Fields[0])
	}
	byName := map[string]formField{}
	for _, f := range form.Fields {
		byName[f.Name] = f
	}
	for _, name := range []string{"kind", "part_of_day", "rich", "out"} {
		if byName[name].Input != "select" {
			t.Errorf("%s drawn as %q, want a select", name, byName[name].Input)
		}
	}
	if f := byName["count"]; f.Input != "number" || f.Step != "1" || strings.Join(f.Hints, ",") != "at least 1" {
		t.Errorf("count = %+v, want a whole number saying at least 1", f)
	}
	if byName["id"].Name != "" || byName[def.SourceTimeField].Name != "" {
		t.Error("the form draws the composed id or source_time")
	}

	payload, ok := form.read(url.Values{
		"f.kind": {"alcohol"}, "f.at": {"2026-10-22T19:30"}, "f.drink": {"red-wine"}, "f.count": {"2"}, "f.abv": {"13.5"},
		"zone": {"Australia/Sydney"}, "offset": {"+11:00"},
	})
	if !ok {
		t.Fatalf("read refused good values: %+v", form.Fields)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got["kind"] != "alcohol" || got["at"] != "2026-10-22 19:30" || got["drink"] != "red-wine" || got["count"] != float64(2) || got["abv"] != 13.5 {
		t.Errorf("payload = %s", payload)
	}
	meal, ok := form.read(url.Values{
		"f.kind": {"meal"}, "f.at": {"2026-10-22T19:30"}, "f.text": {"ate out, rich"}, "f.rich": {"true"}, "f.out": {"false"},
		"zone": {"Australia/Sydney"}, "offset": {"+11:00"},
	})
	if !ok || !strings.Contains(string(meal), `"rich":true`) || !strings.Contains(string(meal), `"out":false`) {
		t.Errorf("meal = %s, ok %v; want rich and out sent as booleans, out false kept", meal, ok)
	}
}
