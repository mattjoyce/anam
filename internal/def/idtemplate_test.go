package def_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/def"
)

// templatedDef is the engine fixture's test.templated, whose identity_template is
// {source}:templated/{/name|slug}/{/at|2006-01-02T15:04}{/source_time/offset?}.
func templatedDef(t *testing.T) def.Definition {
	t.Helper()
	d, ok := catalogtest.Engine.Definitions(t).Lookup("test.templated")
	if !ok {
		t.Fatal("test.templated is not in the engine fixture")
	}
	return d
}

// Card 188: the id is composed from the payload alone. An id the payload carries is
// ignored, and the same payload without it composes the same text.
func TestComposeID_IgnoresTheIDThePayloadCarries(t *testing.T) {
	t.Parallel()
	d := templatedDef(t)
	for _, tc := range []struct{ at, id string }{
		{"2026-10-17 22:00", "conversation:templated/melatonin/2026-10-17T22:00"},
		{"2026-10-18 22:00", "conversation:templated/melatonin/2026-10-18T22:00"},
		{"2026-10-19 07:05", "conversation:templated/melatonin/2026-10-19T07:05"},
	} {
		with := []byte(`{"id": "x", "at": "` + tc.at + `", "name": "melatonin", "amount": 5, "unit": "mg", "note": "to sleep better"}`)
		without := []byte(`{"at": "` + tc.at + `", "name": "melatonin", "amount": 5, "unit": "mg"}`)
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
	d := templatedDef(t)
	cases := []struct {
		name, source, payload, want, wantErr string
	}{
		{"source is the prefix", "workbench", `{"at": "2026-10-17 07:05", "name": "melatonin"}`,
			"workbench:templated/melatonin/2026-10-17T07:05", ""},
		{"slug lower-cases and hyphenates", "conversation", `{"at": "2026-10-17 22:00", "name": "Co-Codamol  500/30"}`,
			"conversation:templated/co-codamol-500-30/2026-10-17T22:00", ""},
		{"missing name", "conversation", `{"at": "2026-10-17 22:00"}`, "", "name"},
		{"name that slugs to nothing", "conversation", `{"at": "2026-10-17 22:00", "name": "!!"}`, "", "empty"},
		{"time not in the fact-time layout", "conversation", `{"at": "2026-10-17T22:00", "name": "melatonin"}`, "", "not a time"},
		{"name is a container", "conversation", `{"at": "2026-10-17 22:00", "name": ["a"]}`, "", "container"},
		{"no source", "", `{"at": "2026-10-17 22:00", "name": "melatonin"}`, "", "source"},
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
	d, _ := catalogtest.Engine.Definitions(t).Lookup("test.measure")
	if _, err := def.ComposeID(d, "withings", []byte(`{}`)); !errors.Is(err, def.ErrNoTemplate) {
		t.Errorf("got %v, want ErrNoTemplate", err)
	}
}

func TestWithID(t *testing.T) {
	t.Parallel()
	d := templatedDef(t)
	const id = "conversation:templated/melatonin/2026-10-17T22:00"
	left := `{"at": "2026-10-17 22:00", "name": "melatonin", "amount": 5, "unit": "mg"}`

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
		in := `{"id": "` + id + `", "at": "2026-10-17 22:00", "name": "melatonin"}`
		out, err := d.WithID("conversation", []byte(in))
		if err != nil || string(out) != in {
			t.Errorf("got %s, %v", out, err)
		}
	})
	for _, bad := range []string{`"conversation:templated/melatonin/2026-10-17T22:30"`, `""`, `7`, `null`} {
		t.Run("disagreeing "+bad, func(t *testing.T) {
			in := `{"id": ` + bad + `, "at": "2026-10-17 22:00", "name": "melatonin"}`
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
		w, _ := catalogtest.Engine.Definitions(t).Lookup("test.measure")
		out, err := w.WithID("withings", []byte(`{"a":1}`))
		if err != nil || string(out) != `{"a":1}` {
			t.Errorf("got %s, %v", out, err)
		}
	})
}

// The parser accepts a Definition without the field (every engine fixture type but
// test.templated) and refuses a template that cannot work.
func TestParse_IdentityTemplate(t *testing.T) {
	t.Parallel()
	manifest := func(key, tpl string) []byte {
		return []byte(`{"meta": {"facet_type": "test.dose-test", "definition_version": 1, "classification": "Normal",
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
