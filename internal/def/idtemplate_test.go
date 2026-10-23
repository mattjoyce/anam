package def_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

// The three melatonin doses on the record (card 188): hand-built ids, written
// through the conversation source before the rule was declared. Whatever the
// Definition declares must compose exactly these.
const (
	melatoninReason = `"reason": "to help adjust to the timezone, and sleep better"`
	dosePayloadFmt  = `{"id": %q, "taken_at": %q, "drug": "melatonin", "amount": 5, "unit": "mg", ` + melatoninReason + `}`
)

func doseDef(t *testing.T) def.Definition {
	t.Helper()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup("health.medication.dose")
	if !ok {
		t.Fatal("health.medication.dose is not bundled")
	}
	return d
}

func TestComposeID_TheLiveMelatoninDoses(t *testing.T) {
	t.Parallel()
	d := doseDef(t)
	for _, tc := range []struct{ takenAt, id string }{
		{"2026-10-17 22:00", "conversation:med/melatonin/dose/2026-10-17T22:00"},
		{"2026-10-18 22:00", "conversation:med/melatonin/dose/2026-10-18T22:00"},
		{"2026-10-19 22:00", "conversation:med/melatonin/dose/2026-10-19T22:00"},
	} {
		// The payload as stored carries its id; the composition ignores it, and the
		// same payload without it composes the same text.
		with := []byte(`{"id": "x", "taken_at": "` + tc.takenAt + `", "drug": "melatonin", "amount": 5, "unit": "mg", ` + melatoninReason + `}`)
		without := []byte(`{"taken_at": "` + tc.takenAt + `", "drug": "melatonin", "amount": 5, "unit": "mg"}`)
		for _, p := range [][]byte{with, without} {
			got, err := def.ComposeID(d, "conversation", p)
			if err != nil || got != tc.id {
				t.Errorf("ComposeID(%s) = %q, %v; want %q", p, got, err, tc.id)
			}
		}
	}
}

func TestComposeID_Cases(t *testing.T) {
	t.Parallel()
	d := doseDef(t)
	cases := []struct {
		name, source, payload, want, wantErr string
	}{
		{"source is the prefix", "workbench", `{"taken_at": "2026-10-17 07:05", "drug": "melatonin"}`,
			"workbench:med/melatonin/dose/2026-10-17T07:05", ""},
		{"slug lower-cases and hyphenates", "conversation", `{"taken_at": "2026-10-17 22:00", "drug": "Co-Codamol  500/30"}`,
			"conversation:med/co-codamol-500-30/dose/2026-10-17T22:00", ""},
		{"missing drug", "conversation", `{"taken_at": "2026-10-17 22:00"}`, "", "drug"},
		{"drug that slugs to nothing", "conversation", `{"taken_at": "2026-10-17 22:00", "drug": "!!"}`, "", "empty"},
		{"time not in the fact-time layout", "conversation", `{"taken_at": "2026-10-17T22:00", "drug": "melatonin"}`, "", "not a time"},
		{"drug is a container", "conversation", `{"taken_at": "2026-10-17 22:00", "drug": ["a"]}`, "", "container"},
		{"no source", "", `{"taken_at": "2026-10-17 22:00", "drug": "melatonin"}`, "", "source"},
	}
	for _, tc := range cases {
		got, err := def.ComposeID(d, tc.source, []byte(tc.payload))
		switch {
		case tc.wantErr == "" && (err != nil || got != tc.want):
			t.Errorf("%s: got %q, %v; want %q", tc.name, got, err, tc.want)
		case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr) || !errors.Is(err, def.ErrNotCompliant)):
			t.Errorf("%s: got %q, %v; want ErrNotCompliant mentioning %q", tc.name, got, err, tc.wantErr)
		}
	}
}

func TestComposeID_NoTemplate(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, _ := reg.Lookup("health.weight")
	if _, err := def.ComposeID(d, "withings", []byte(`{}`)); !errors.Is(err, def.ErrNoTemplate) {
		t.Errorf("got %v, want ErrNoTemplate", err)
	}
}

func TestWithID(t *testing.T) {
	t.Parallel()
	d := doseDef(t)
	const id = "conversation:med/melatonin/dose/2026-10-17T22:00"
	left := `{"taken_at": "2026-10-17 22:00", "drug": "melatonin", "amount": 5, "unit": "mg"}`

	t.Run("omitted is filled", func(t *testing.T) {
		out, err := d.WithID("conversation", []byte(left))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), `"id":"`+id+`"`) || !strings.Contains(string(out), `"amount":5`) {
			t.Errorf("got %s", out)
		}
	})
	t.Run("matching is untouched", func(t *testing.T) {
		in := `{"id": "` + id + `", "taken_at": "2026-10-17 22:00", "drug": "melatonin"}`
		out, err := d.WithID("conversation", []byte(in))
		if err != nil || string(out) != in {
			t.Errorf("got %s, %v", out, err)
		}
	})
	for _, bad := range []string{`"conversation:med/melatonin/dose/2026-10-17T22:30"`, `""`, `7`, `null`} {
		t.Run("disagreeing "+bad, func(t *testing.T) {
			in := `{"id": ` + bad + `, "taken_at": "2026-10-17 22:00", "drug": "melatonin"}`
			_, err := d.WithID("conversation", []byte(in))
			if !errors.Is(err, def.ErrIdentityField) || !strings.Contains(err.Error(), bad) || !strings.Contains(err.Error(), id) {
				t.Errorf("got %v, want ErrIdentityField showing %s and %s", err, bad, id)
			}
		})
	}
	t.Run("not an object", func(t *testing.T) {
		if _, err := d.WithID("conversation", []byte(`[1]`)); !errors.Is(err, def.ErrNotCompliant) {
			t.Errorf("got %v", err)
		}
	})
	t.Run("no template leaves the payload", func(t *testing.T) {
		reg, _ := def.Bundled()
		w, _ := reg.Lookup("health.weight")
		out, err := w.WithID("withings", []byte(`{"a":1}`))
		if err != nil || string(out) != `{"a":1}` {
			t.Errorf("got %s, %v", out, err)
		}
	})
}

// The parser accepts a Definition without the field (every bundled type but the
// dose) and refuses a template that cannot work.
func TestParse_IdentityTemplate(t *testing.T) {
	t.Parallel()
	manifest := func(key, tpl string) []byte {
		return []byte(`{"meta": {"facet_type": "health.dose-test", "definition_version": 1, "classification": "Normal",
		  "validation_mode": "strict", "source_schema": "t/x",
		  "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "UTC"}},
		  "identity_key": ` + key + `, "identity_template": "` + tpl + `"},
		  "schema": {"type": "object", "required": ["id", "at", "drug"], "additionalProperties": false,
		    "properties": {"id": {"type": "string", "minLength": 1}, "at": {"type": "string", "minLength": 1},
		      "drug": {"type": "string", "minLength": 1}, "note": {"type": "string", "minLength": 1}}}}`)
	}
	for _, tc := range []struct {
		name, key, tpl, wantErr string
	}{
		{"valid", `["/id"]`, `{source}:{/drug|slug}/{/at|2006-01-02}`, ""},
		{"plain pointer", `["/id"]`, `{source}:{/drug}`, ""},
		{"two identity fields", `["/id", "/drug"]`, `{source}:{/at}`, "one top-level pointer"},
		{"nested identity field", `["/a/id"]`, `{source}:{/drug}`, "one top-level pointer"},
		{"reads its own field", `["/id"]`, `{source}:{/id}`, "the field it composes"},
		{"reads nothing", `["/id"]`, `fixed`, "reads no payload field"},
		{"unclosed", `["/id"]`, `{source}:{/drug`, "never closed"},
		{"stray close", `["/id"]`, `{source}:}`, "no '{'"},
		{"unknown transform", `["/id"]`, `{/drug|upper}`, "neither slug nor a time layout"},
		{"time layout on another field", `["/id"]`, `{/note|2006-01-02}`, "fact-time"},
		{"pointer without slash", `["/id"]`, `{drug}`, "must start with"},
		{"field the schema does not require", `["/id"]`, `{/note}`, "must be required"},
		{"transform on source", `["/id"]`, `{source|slug}:{/drug}`, "no transform"},
	} {
		_, err := def.Parse(manifest(tc.key, tc.tpl))
		switch {
		case tc.wantErr == "" && err != nil:
			t.Errorf("%s: %v", tc.name, err)
		case tc.wantErr != "" && (err == nil || !errors.Is(err, def.ErrBadManifest) || !strings.Contains(err.Error(), tc.wantErr)):
			t.Errorf("%s: got %v, want ErrBadManifest mentioning %q", tc.name, err, tc.wantErr)
		}
	}
}

// The types that declare an id rule: the dose, the intake and the three attention types,
// which take the same moment-and-offset shape. Any other bundled type lets the writer choose
// the identity field's text.
func TestBundled_OnlyTheseTypesDeclareATemplate(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range reg.All() {
		want := d.Type == "health.medication.dose" || d.Type == "health.intake" || strings.HasPrefix(string(d.Type), "mind.attention")
		if got := d.IdentityTemplate != ""; got != want {
			t.Errorf("%s declares a template: %v, want %v", d.Type, got, want)
		}
	}
	if d := doseDef(t); d.Version != 2 {
		t.Errorf("health.medication.dose is v%d, want 2", d.Version)
	}
}
