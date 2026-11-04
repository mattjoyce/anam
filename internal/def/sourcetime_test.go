package def_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
)

// clockManifest is a reading timed by a wall clock in zone, laid out as layout,
// carrying source_time as shape (v1 unless a test changes it).
func clockManifest(t *testing.T, layout, zone, shape string) string {
	t.Helper()
	if shape == "" {
		raw, err := os.ReadFile("sourcetime/v1.json")
		if err != nil {
			t.Fatal(err)
		}
		shape = string(raw)
	}
	return `{
  "meta": {
    "facet_type": "test.raw-reading", "definition_version": 1, "classification": "Normal",
    "validation_mode": "strict", "source_schema": "test/reading",
    "time_semantics": {"fact_time": {"pointer": "/at", "layout": "` + layout + `", "zone": "` + zone + `"}},
    "identity_key": ["/at"]
  },
  "schema": {
    "type": "object", "required": ["at", "source_time"], "additionalProperties": false,
    "properties": {"at": {"type": "string", "minLength": 1}, "source_time": ` + shape + `}
  }
}`
}

func reading(at, zone, basis string) string {
	return `{"at": "` + at + `", "source_time": {"value": "` + at + `", "zone": "` + zone + `", "basis": "` + basis + `"}}`
}

// Acceptance example 1: an event supplied with its offset is placed at its UTC
// instant, and the value and basis it was read by stay in the payload.
func TestSourceTime_AnOffsetValueIsItsInstantAndKeepsHowItWasRead(t *testing.T) {
	t.Parallel()
	d := mustParse(t, clockManifest(t, "2006-01-02T15:04:05Z07:00", "UTC", ""))
	doc := mustDecode(t, reading("2026-10-20T20:00:00+11:00", "+11:00", "source"))
	if err := d.Check(doc); err != nil {
		t.Fatalf("Check: %v", err)
	}
	got, err := d.FactTimeOf(doc)
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("fact time %v, want %v", got, want)
	}
}

// Acceptance example 4: a wall clock the zone skips or repeats is refused, never
// moved or given a side; one that exists once, and one whose offset says which side
// it means, are admitted.
func TestSourceTime_AWallClockTheZoneSkipsOrRepeatsIsRefused(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, layout, zone, at string
		want                   string // "" admits; else a word the refusal says
	}{
		{"Sydney's clocks skip 02:30 on 4 October", "2006-01-02 15:04", "Australia/Sydney", "2026-10-04 02:30", "skip"},
		{"Sydney's clocks repeat 02:30 on 5 April", "2006-01-02 15:04", "Australia/Sydney", "2026-04-05 02:30", "twice"},
		{"London's clocks skip 01:30 on 29 March", "2006-01-02 15:04", "Europe/London", "2026-03-29 01:30", "skip"},
		{"London's clocks repeat 01:30 on 25 October", "2006-01-02 15:04", "Europe/London", "2026-10-25 01:30", "twice"},
		{"an hour after Sydney's repeat is once", "2006-01-02 15:04", "Australia/Sydney", "2026-04-05 03:30", ""},
		{"an hour before Sydney's skip is once", "2006-01-02 15:04", "Australia/Sydney", "2026-10-04 01:30", ""},
		{"a Sydney midnight on a changing day is once", "2006-01-02", "Australia/Sydney", "2026-10-04", ""},
		{"an offset says which 02:30 it means", "2006-01-02T15:04:05Z07:00", "Australia/Sydney", "2026-04-05T02:30:00+10:00", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			d := mustParse(t, clockManifest(t, c.layout, c.zone, ""))
			_, err := d.FactTimeOf(mustDecode(t, reading(c.at, c.zone, "configuration")))
			switch {
			case c.want == "" && err != nil:
				t.Errorf("refused: %v", err)
			case c.want != "" && (!errors.Is(err, def.ErrNotCompliant) || !strings.Contains(err.Error(), c.want)):
				t.Errorf("err = %v, want a refusal saying %q", err, c.want)
			}
		})
	}
}

func TestSourceTime_AZoneThatIsNoZoneIsRefused(t *testing.T) {
	t.Parallel()
	d := mustParse(t, clockManifest(t, "2006-01-02T15:04:05Z07:00", "UTC", ""))
	for zone, ok := range map[string]bool{
		"Europe/London": true, "UTC": true, "+01:00": true, "-03:30": true, "+14:00": true,
		"Europe/Londn": false, "Local": false, "+15:00": false, "Mars/Olympus": false,
	} {
		err := d.Check(mustDecode(t, reading("2026-10-20T08:00:00Z", zone, "source")))
		if ok != (err == nil) {
			t.Errorf("zone %q: err = %v, want admitted %v", zone, err, ok)
		}
		if err != nil && !errors.Is(err, def.ErrNotCompliant) {
			t.Errorf("zone %q: err = %v, want ErrNotCompliant", zone, err)
		}
	}
	if err := d.Check(mustDecode(t, reading("2026-10-20T08:00:00Z", "UTC", "guessed"))); err == nil {
		t.Error("a basis that is none of the three was admitted")
	}
}

// The shape is copied into each Definition, so Parse is what keeps the copies one
// shape: prose may differ, a rule may not.
func TestSourceTime_ADefinitionCarriesTheShapeOrIsRefused(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("sourcetime/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	v1 := string(raw)
	reworded := strings.Replace(v1, `"description": "Who decided the zone.`, `"description": "Garmin's own word. Who decided the zone.`, 1)
	if reworded == v1 {
		t.Fatal("the rewording changed nothing")
	}
	mustParse(t, clockManifest(t, "2006-01-02T15:04:05Z07:00", "UTC", reworded))
	for name, shape := range map[string]string{
		"a fourth basis":  strings.Replace(v1, `"assumption"]`, `"assumption", "hearsay"]`, 1),
		"a field more":    strings.Replace(v1, `"properties": {`, `"properties": {"note": {"type": "string"},`, 1),
		"zone optional":   strings.Replace(v1, `["value", "zone", "basis"]`, `["value", "basis"]`, 1),
		"an empty object": `{"type": "object", "additionalProperties": false, "properties": {}}`,
	} {
		_, err := def.Parse([]byte(clockManifest(t, "2006-01-02T15:04:05Z07:00", "UTC", shape)))
		if !errors.Is(err, def.ErrBadManifest) {
			t.Errorf("%s: err = %v, want ErrBadManifest", name, err)
		}
	}
}

// The engine fixture's types carry the shapes as a catalog writes them, pretty-printed
// inside a whole manifest: test.measure a copy of v1 and test.reading a copy of v2 (the
// offset). Each loads as it is, and the same file with one rule of its copy changed is
// refused when it loads, so no catalog can carry a third shape.
func TestSourceTime_TheFixturesCopiesAreTheShapes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		file, shape string
		edits       map[string][2]string // name: old, new in the manifest's bytes
	}{
		{"test.measure.json", "v1.json", map[string][2]string{
			"a fourth basis": {`"assumption"`, `"assumption", "hearsay"`},
			"zone optional":  {"\"required\": [\n          \"value\",\n          \"zone\",", "\"required\": [\n          \"value\","},
			"an offset more": {"\"basis\": {\n            \"type\"", "\"offset\": {\"type\": \"string\"},\n          \"basis\": {\n            \"type\""},
		}},
		{"test.reading.json", "v2.json", map[string][2]string{
			"a fourth basis":  {`"assumption"`, `"assumption", "hearsay"`},
			"offset optional": {"\"zone\",\n          \"offset\",\n          \"basis\"", "\"zone\",\n          \"basis\""},
			"a zone unchecked": {`"minLength": 1,
            "pattern": "^([A-Za-z]`, `"minLength": 1,
            "format": "^([A-Za-z]`},
		}},
	} {
		raw, err := fs.ReadFile(fixtureDefs(), tc.file)
		if err != nil {
			t.Fatal(err)
		}
		mustParse(t, string(raw))
		shape, err := os.ReadFile("sourcetime/" + tc.shape)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Schema struct {
				Required   []string       `json:"required"`
				Properties map[string]any `json:"properties"`
			} `json:"schema"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(m.Schema.Required, def.SourceTimeField) {
			t.Errorf("%s does not require %s", tc.file, def.SourceTimeField)
		}
		_, offset := m.Schema.Properties[def.SourceTimeField].(map[string]any)["properties"].(map[string]any)["offset"]
		if want := strings.Contains(string(shape), `"offset"`); offset != want {
			t.Errorf("%s carries an offset %v, want the %s shape's %v", tc.file, offset, tc.shape, want)
		}
		for name, e := range tc.edits {
			edited := strings.Replace(string(raw), e[0], e[1], 1)
			if edited == string(raw) {
				t.Fatalf("%s, %s: the edit changed nothing", tc.file, name)
			}
			if _, err := def.Parse([]byte(edited)); !errors.Is(err, def.ErrBadManifest) {
				t.Errorf("%s, %s: Parse = %v, want ErrBadManifest", tc.file, name, err)
			}
		}
	}
}
