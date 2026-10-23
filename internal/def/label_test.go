package def_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// withLabel is the test manifest with a handling block holding the label.
func withLabel(label string) string {
	return strings.Replace(bpManifest, `"schema"`, `"handling": {"guidance": "sitting", "label": `+label+`}, "schema"`, 1)
}

// Card 199, slice 3: a label is literal text with {/pointer} fields, and it never
// fails. A missing field renders as nothing, a text field as its first line, and a
// label whose fields all came out empty is empty so the page falls back.
func TestLabel_Renders(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, label, payload, want string
	}{
		{"two fields and text", `"{/systolic}/{/diastolic} mmHg"`, `{"systolic": 118, "diastolic": 76}`, "118/76 mmHg"},
		{"a missing field is nothing", `"{/systolic}/{/pulse} bpm"`, `{"systolic": 118}`, "118/ bpm"},
		{"spaces collapse around a missing field", `"{/position} {/systolic} {/comment}"`, `{"systolic": 118}`, "118"},
		{"a text field is its first line", `"{/comment}"`, `{"comment": "Before breakfast\nthen a walk"}`, "Before breakfast"},
		{"a string and a number", `"{/position} at {/pulse}"`, `{"position": "sitting", "pulse": 61.5}`, "sitting at 61.5"},
		{"every field empty is no label", `"{/position} {/comment}"`, `{"systolic": 118}`, ""},
		{"a null is empty", `"{/position}"`, `{"position": null}`, ""},
		{"an object is empty", `"x{/position}"`, `{"position": {"a": 1}}`, ""},
		{"a payload that is not JSON", `"{/position}"`, `not json`, ""},
		{"a pointer through an array", `"{/tags/0}"`, `{"tags": ["a", "b"]}`, "a"},
		{"literal text is kept as written", `"BP ({/systolic}) · {/diastolic}"`, `{"systolic": 1, "diastolic": 2}`, "BP (1) · 2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			manifest := withLabel(c.label)
			if strings.Contains(c.label, "/tags/0") {
				manifest = strings.Replace(manifest, `"comment": {"type"`, `"tags": {"type": "array", "items": {"type": "string"}}, "comment": {"type"`, 1)
			}
			d := mustParse(t, manifest)
			if got := d.Label([]byte(c.payload)); got != c.want {
				t.Errorf("Label(%s) = %q, want %q", c.payload, got, c.want)
			}
		})
	}
}

func TestLabel_NoneDeclared(t *testing.T) {
	t.Parallel()
	d := mustParse(t, bpManifest)
	if d.LabelTemplate != "" || d.Label([]byte(bpPayload)) != "" {
		t.Errorf("a Definition with no handling has label %q", d.Label([]byte(bpPayload)))
	}
	g := mustParse(t, strings.Replace(bpManifest, `"schema"`, `"handling": {"guidance": "g"}, "schema"`, 1))
	if g.Label([]byte(bpPayload)) != "" {
		t.Error("a handling block without a label has one")
	}
}

// A label that cannot work is refused when the Definition loads, not drawn blank.
func TestLabel_ParseRefusals(t *testing.T) {
	t.Parallel()
	cases := []struct{ name, label, want string }{
		{"not a string", `7`, "non-empty string"},
		{"empty", `""`, "non-empty string"},
		{"reads no field", `"a reading"`, "reads no payload field"},
		{"a field the schema lacks", `"{/systolik}"`, "systolik"},
		{"source is not a label field", `"{source}"`, "JSON Pointer"},
		{"a transform", `"{/systolic|slug}"`, "no transform"},
		{"an optional mark", `"{/systolic?}"`, "no transform and no optional mark"},
		{"unclosed", `"{/systolic"`, "never closed"},
		{"stray close", `"{/systolic}}"`, "no '{'"},
		{"nested", `"{/sys{/dia}}"`, "inside a field"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			_, err := def.Parse([]byte(withLabel(c.label)))
			if !errors.Is(err, def.ErrBadManifest) || !strings.Contains(err.Error(), c.want) {
				t.Errorf("Parse = %v, want ErrBadManifest saying %q", err, c.want)
			}
		})
	}
}

// The bundled Definitions that declare a label render these examples as shown on the
// picker: synthetic payloads, one per shape.
func TestLabel_BundledOnesRenderTheirOwnExamples(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ typ, payload, want string }{
		{"health.weight", `{"id": "w1", "date": "2026-10-02", "kg": 80.4}`, "80.4 kg"},
		{"health.medication.dose", `{"taken_at": "2026-10-17 22:00", "drug": "melatonin", "amount": 5, "unit": "mg"}`, "melatonin 5 mg"},
		{"mind.attention", `{"at": "2026-10-17 22:00", "topic": "deskcam", "intensity": 3}`, "deskcam"},
		{"health.intake", `{"kind": "caffeine", "at": "2026-10-17 08:00", "drink": "flat-white", "count": 1}`, "caffeine flat-white"},
		{"health.intake", `{"kind": "meal", "at": "2026-10-17 19:00", "text": "pasta, out with friends"}`, "meal pasta, out with friends"},
		{"record.edge", `{"at": "2026-10-21 14:30", "text": "During a large work meeting.\nPresenting."}`, "During a large work meeting."},
	}
	for _, c := range cases {
		d, ok := reg.Lookup(facet.Type(c.typ))
		if !ok {
			t.Fatalf("%s is not bundled", c.typ)
		}
		if got := d.Label([]byte(c.payload)); got != c.want {
			t.Errorf("%s label of %s = %q, want %q", c.typ, c.payload, got, c.want)
		}
	}
}
