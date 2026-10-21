package inspect

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/def"
)

// The entry form is drawn from a Definition and read back through the same walk
// (card 188, S4, review finding 11): a field the page draws is a field it reads, as
// the type JSON Schema declares it. Nothing here names a type. What it may take from
// the Definition is the schema's properties (type, required, enum, description,
// examples), the fact-time field's layout and zone, the identity pointers (which fields
// lock when a facet is corrected), and the version it drew. What it must not do is
// compose the id, read a rule out of prose, or decide what may be entered: the gate
// fills the identity field, and the Workbench principal's types are the list.

// formOption is one choice of a select.
type formOption struct {
	Text     string
	JSON     string
	Selected bool
}

// formField is one input of the form.
type formField struct {
	Name, Pointer string
	// Input is the HTML control: text, number, date, datetime-local or select.
	Input string
	// Step is the number input's step: "any" for a number, "1" for an integer.
	Step        string
	Kind        string // the JSON type: string, number, integer or boolean
	Required    bool
	Locked      bool
	FactTime    bool // the fact-time field, whose wall clock the page reads an offset for
	Description string
	Placeholder string
	Hints       []string
	Options     []formOption
	Value       string
	Problem     string
}

// entryForm is a Definition drawn as a form.
type entryForm struct {
	Fields []formField
	// loc and layout are the fact-time field's zone and layout.
	loc    *time.Location
	layout string
	timeAt string
	// Zoned says the Definition's fact time reads the zone a fact names (card 198), so
	// the form sends source_time without drawing it: Zone and Offset are hidden
	// fields the page fills from the browser, or from the facet being corrected.
	Zoned         bool
	Zone, Offset  string
	OffsetProblem string
}

// timeInput is the layout a datetime-local control writes, and dateInput a date's.
const (
	timeInput = "2006-01-02T15:04"
	dateInput = "2006-01-02"
)

// errNotDrawn says why a Definition has no form, in words a person can act on.
type errNotDrawn string

func (e errNotDrawn) Error() string { return string(e) }

// drawForm draws the payload schema in body as a form, with the Definition's identity
// fields marked to lock when lock is set. A schema it cannot draw (a nested object, a
// list, a choice between shapes) is refused with a message naming the fields.
func drawForm(body []byte, d def.Definition, lock bool) (entryForm, error) {
	var m struct {
		Schema map[string]any `json:"schema"`
	}
	if err := json.Unmarshal(body, &m); err != nil || len(m.Schema) == 0 {
		return entryForm{}, errNotDrawn("This type declares no payload schema, so there are no fields to draw.")
	}
	props, _ := m.Schema["properties"].(map[string]any)
	if len(props) == 0 {
		return entryForm{}, errNotDrawn("This type's payload schema lists no fields, so there are none to draw.")
	}
	loc, err := time.LoadLocation(d.FactTime.Zone)
	if err != nil {
		return entryForm{}, errNotDrawn(fmt.Sprintf("The fact-time zone %q cannot be loaded: %v.", d.FactTime.Zone, err))
	}
	form := entryForm{timeAt: d.FactTime.Pointer, layout: d.FactTime.Layout, loc: loc, Zoned: d.FactTime.ZonePointer != ""}
	required := stringSet(m.Schema["required"])
	locked := map[string]bool{}
	if lock {
		for _, p := range d.IdentityFields() {
			locked[p] = true
		}
	}
	var undrawn []string
	for _, name := range orderedNames(props, required, strings.TrimPrefix(d.FactTime.Pointer, "/")) {
		ptr := "/" + name
		if ptr == d.ComposedField() {
			continue // the gate composes it from the Definition's rule
		}
		// source_time is the form's to fill, from the browser's zone, when the fact
		// time reads its zone; an optional one the fact time does not read is not
		// sent. A required one with no zone pointer falls through and is refused.
		if name == def.SourceTimeField && (form.Zoned || !required[name]) {
			continue
		}
		sub, _ := props[name].(map[string]any)
		field, why := form.draw(name, sub, required[name])
		if why != "" {
			undrawn = append(undrawn, fmt.Sprintf("%s (%s)", name, why))
			continue
		}
		field.Locked = locked[ptr]
		form.Fields = append(form.Fields, field)
	}
	if len(undrawn) > 0 {
		return entryForm{}, errNotDrawn("A form cannot be drawn for this type yet: it has fields that are not single values, " +
			strings.Join(undrawn, ", ") + ". Nested objects and lists are not drawn.")
	}
	return form, nil
}

// stringSet reads a schema's list of names (its `required`) as a set.
func stringSet(v any) map[string]bool {
	out := map[string]bool{}
	list, _ := v.([]any)
	for _, item := range list {
		if name, ok := item.(string); ok {
			out[name] = true
		}
	}
	return out
}

// orderedNames puts the fact-time field first, then the required fields, then the
// rest, each alphabetical: a decoded JSON object has no order.
func orderedNames(props map[string]any, required map[string]bool, first string) []string {
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	rank := func(n string) int {
		switch {
		case n == first:
			return 0
		case required[n]:
			return 1
		}
		return 2
	}
	sort.Slice(names, func(i, j int) bool {
		if ri, rj := rank(names[i]), rank(names[j]); ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	return names
}

// draw turns one property into a field, or says why it cannot be one.
func (f entryForm) draw(name string, sub map[string]any, required bool) (formField, string) {
	for _, k := range []string{"oneOf", "anyOf", "allOf", "$ref", "if", "not"} {
		if _, ok := sub[k]; ok {
			return formField{}, "uses " + k
		}
	}
	kind := singleType(sub)
	enum, hasEnum := sub["enum"].([]any)
	if !hasEnum && kind == "" {
		return formField{}, "no single type"
	}
	switch kind {
	case "object":
		return formField{}, "an object"
	case "array":
		return formField{}, "a list"
	case "null":
		return formField{}, "null"
	}
	field := formField{Name: name, Pointer: "/" + name, Kind: kind, Required: required, Input: "text"}
	field.Description, _ = sub["description"].(string)
	if ex, ok := sub["examples"].([]any); ok && len(ex) > 0 {
		field.Placeholder = fmt.Sprint(ex[0])
	}
	field.Hints = hintsOf(sub)
	switch {
	case hasEnum:
		field.Input = "select"
		field.Options = optionsOf(enum, !required)
	case kind == "boolean":
		field.Input = "select"
		field.Options = []formOption{{}, {Text: "true", JSON: "true"}, {Text: "false", JSON: "false"}}
	case kind == "number":
		field.Input, field.Step = "number", "any"
	case kind == "integer":
		field.Input, field.Step = "number", "1"
	case field.Pointer == f.timeAt && kind == "string" && f.Zoned:
		field.FactTime = true
		field.Input = timeControl(f.layout)
		field.Hints = append(field.Hints, "read in your browser's zone, or as "+f.loc.String()+" time without one")
	case field.Pointer == f.timeAt && kind == "string":
		field.Input = timeControl(f.layout)
		field.Hints = append(field.Hints, "read as "+f.loc.String()+" time")
	}
	return field, ""
}

// singleType names a property's one JSON type, or "" when it has none or several.
func singleType(sub map[string]any) string {
	types := def.TypesOf(sub)
	if len(types) != 1 {
		return ""
	}
	return types[0]
}

// timeControl is the control that fits a fact-time layout: a date when the layout
// shows no clock, a date and time when it does.
func timeControl(layout string) string {
	a := time.Date(2001, 2, 3, 0, 0, 0, 0, time.UTC).Format(layout)
	b := time.Date(2001, 2, 3, 16, 5, 6, 0, time.UTC).Format(layout)
	if a == b {
		return "date"
	}
	return "datetime-local"
}

// hintsOf states a property's bounds as page hints. The gate checks them again, so a
// hint here is help and not a rule.
func hintsOf(sub map[string]any) []string {
	var hints []string
	for _, h := range []struct{ key, text string }{
		{"minLength", "at least %v characters"}, {"maxLength", "at most %v characters"},
		{"exclusiveMinimum", "greater than %v"}, {"minimum", "at least %v"},
		{"exclusiveMaximum", "less than %v"}, {"maximum", "at most %v"},
	} {
		if v, ok := sub[h.key]; ok {
			hints = append(hints, fmt.Sprintf(h.text, v))
		}
	}
	return hints
}

// optionsOf lists an enum as select options, with a blank first when the field is
// optional.
func optionsOf(values []any, blank bool) []formOption {
	var out []formOption
	if blank {
		out = append(out, formOption{})
	}
	for _, v := range values {
		raw, _ := json.Marshal(v) // a decoded JSON value always marshals
		text := fmt.Sprint(v)
		out = append(out, formOption{Text: text, JSON: string(raw)})
	}
	return out
}

// fill sets each field from a stored payload: the values a correction starts from. A
// stored field the form does not draw (the composed id) is left alone.
func (f *entryForm) fill(payload []byte) error {
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(payload, &stored); err != nil {
		return fmt.Errorf("the stored payload is not an object: %w", err)
	}
	for i := range f.Fields {
		fld := &f.Fields[i]
		raw, ok := stored[fld.Name]
		if !ok {
			continue
		}
		fld.Value = f.textOf(fld, raw)
	}
	// A correction keeps the zone the fact was said in, and its offset, which a dose's
	// id reads: corrected from elsewhere, it is still the same fact.
	if raw, ok := stored[def.SourceTimeField]; ok && f.Zoned {
		var st struct{ Zone, Offset string }
		if json.Unmarshal(raw, &st) == nil {
			f.Zone, f.Offset = st.Zone, st.Offset
		}
	}
	return nil
}

// textOf is how a stored JSON value reads in its input.
func (f *entryForm) textOf(fld *formField, raw json.RawMessage) string {
	var s string
	text := strings.TrimSpace(string(raw))
	if json.Unmarshal(raw, &s) == nil {
		text = s
	}
	if fld.Pointer == f.timeAt && (fld.Input == "datetime-local" || fld.Input == "date") {
		t, err := time.ParseInLocation(f.layout, text, f.wallLoc())
		if err != nil {
			fld.Problem = fmt.Sprintf("the stored value %q is not in the layout %q", text, f.layout)
			return ""
		}
		if fld.Input == "date" {
			return t.Format(dateInput)
		}
		return t.Format(timeInput)
	}
	if fld.Input == "select" {
		for _, o := range fld.Options {
			if o.JSON == string(raw) {
				return o.Text
			}
		}
	}
	return text
}

// read takes the posted values into the fields and builds the payload from them. A
// value that is not what its field takes is a Problem beside that field, and the
// payload is not built. Empty optional fields are left out, and a required one left
// empty is left out too: the gate names it in its own words.
func (f *entryForm) read(vals url.Values) (json.RawMessage, bool) {
	payload := map[string]json.RawMessage{}
	ok := true
	for i := range f.Fields {
		fld := &f.Fields[i]
		fld.Value = vals.Get("f." + fld.Name)
		fld.Problem = ""
		fld.Value = strings.TrimSpace(fld.Value)
		if fld.Value == "" {
			continue
		}
		raw, err := f.encode(fld)
		if err != nil {
			fld.Problem = err.Error()
			ok = false
			continue
		}
		payload[fld.Name] = raw
	}
	if !ok {
		return nil, false
	}
	st, ok := f.sourceTime(vals, payload)
	if !ok {
		return nil, false
	}
	if st != nil {
		payload[def.SourceTimeField] = st
	}
	out, err := json.Marshal(payload)
	if err != nil {
		return nil, false // a map of raw JSON values always marshals
	}
	return out, true
}

// encode writes one posted value as the JSON its field takes.
func (f *entryForm) encode(fld *formField) (json.RawMessage, error) {
	switch {
	case fld.Input == "select":
		for _, o := range fld.Options {
			if o.Text == fld.Value && o.JSON != "" {
				return json.RawMessage(o.JSON), nil
			}
		}
		return nil, errors.New("not one of the choices")
	case fld.Input == "date" || fld.Input == "datetime-local":
		return f.encodeTime(fld)
	case fld.Kind == "number":
		n, err := strconv.ParseFloat(fld.Value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("%q is not a number", fld.Value)
		}
		return json.RawMessage(strconv.FormatFloat(n, 'f', -1, 64)), nil
	case fld.Kind == "integer":
		n, err := strconv.ParseInt(fld.Value, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("%q is not a whole number", fld.Value)
		}
		return json.RawMessage(strconv.FormatInt(n, 10)), nil
	}
	return json.Marshal(fld.Value)
}

// encodeTime reads the control's value as a time and writes it in the Definition's
// layout, so the page never guesses a format. A zoneless layout keeps the wall clock's
// digits as typed, which the gate reads in the zone the fact names; a layout with an
// offset writes the Definition zone's.
func (f *entryForm) encodeTime(fld *formField) (json.RawMessage, error) {
	in := []string{timeInput, timeInput + ":05"}
	if fld.Input == "date" {
		in = []string{dateInput}
	}
	for _, layout := range in {
		if t, err := time.ParseInLocation(layout, fld.Value, f.wallLoc()); err == nil {
			return json.Marshal(t.Format(f.layout))
		}
	}
	return nil, fmt.Errorf("%q is not a date and time", fld.Value)
}
