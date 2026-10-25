package def_test

import (
	"encoding/json"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
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
    "facet_type": "health.reading", "definition_version": 1, "classification": "Normal",
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

// bareDate reports whether a time layout names a day and no time of day. A date stays a
// local date (card 198): it has no clock and no offset to state, so a type timed by one is
// exempt from the rule below and needs no source_time.
func bareDate(layout string) bool {
	at := time.Date(2001, 2, 3, 16, 5, 6, 0, time.UTC)
	return at.Format(layout) == at.Add(-12*time.Hour).Format(layout)
}

// readsALocalClock are the bundled types whose fact time is a wall clock read in the
// Definition's zone, and which do not yet say how it was read. It is empty (card 198): every
// type that reads a local clock requires source_time. A type timed by a bare date is exempt by
// that card's rule. A new type that reads a local clock must require source_time from its first
// version, so this list does not grow.
var readsALocalClock = map[string]bool{}

func TestSourceTime_EveryTypeReadingALocalClockSaysHow(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]bool{}
	for _, s := range stored {
		if d, ok := reg.Lookup(s.Type); !ok || d.Version != s.Version {
			continue
		}
		var m struct {
			Schema struct {
				Required []string `json:"required"`
			} `json:"schema"`
		}
		if err := json.Unmarshal(s.Body, &m); err != nil {
			t.Fatal(err)
		}
		required[string(s.Type)] = slices.Contains(m.Schema.Required, def.SourceTimeField)
	}
	for _, d := range reg.All() {
		local := d.FactTime.Zone != "UTC" && !strings.Contains(d.FactTime.Layout, "07") && !strings.Contains(d.FactTime.Layout, "Z") &&
			!bareDate(d.FactTime.Layout)
		listed := readsALocalClock[string(d.Type)]
		switch {
		case local && !required[string(d.Type)] && !listed:
			t.Errorf("%s reads %q in %s and does not require %s", d.Type, d.FactTime.Layout, d.FactTime.Zone, def.SourceTimeField)
		case listed && (required[string(d.Type)] || !local):
			t.Errorf("%s is listed as not yet saying how its clock was read, and it says how or reads no clock: take it off the list", d.Type)
		}
	}
	for typ := range readsALocalClock {
		if _, ok := reg.Lookup(facet.Type(typ)); !ok {
			t.Errorf("%s is listed and not bundled: take it off the list", typ)
		}
	}
}

// Card 198: the importers' types say how their clocks were read. A type whose every
// writer sends source_time requires it; one whose source has no export left to extract
// again declares it without requiring it, so its old payloads still admit.
func TestSourceTime_TheImportersTypesCarryIt(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	for typ, want := range map[facet.Type]struct {
		version  int
		required bool
	}{
		"activity.exercise-session.concept2":  {2, true},
		"activity.exercise-session.strong":    {2, true},
		"record.document":                     {2, true},
		"activity.exercise-session.technogym": {2, false},
		"health.blood-pressure":               {8, true},
		"health.lab-result":                   {3, true},
		"health.lab-panel":                    {3, true},
	} {
		d, ok := reg.Lookup(typ)
		if !ok || d.Version != want.version {
			t.Errorf("%s = v%d, %v; want v%d", typ, d.Version, ok, want.version)
			continue
		}
		for _, s := range stored {
			if s.Type != typ || s.Version != d.Version {
				continue
			}
			var m struct {
				Schema struct {
					Required   []string                   `json:"required"`
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"schema"`
			}
			if err := json.Unmarshal(s.Body, &m); err != nil {
				t.Fatal(err)
			}
			if _, ok := m.Schema.Properties[def.SourceTimeField]; !ok {
				t.Errorf("%s v%d does not declare %s", typ, d.Version, def.SourceTimeField)
			}
			if got := slices.Contains(m.Schema.Required, def.SourceTimeField); got != want.required {
				t.Errorf("%s v%d requires %s = %v, want %v", typ, d.Version, def.SourceTimeField, got, want.required)
			}
		}
	}
}

// Card 198, slice A: five types no writer sends any more require source_time, so a
// facet written again says how its clock was read. Four are the health ETL's period
// summaries, retired with health-db at card 55; activity.gym-visit's only writer was the
// Strong extract, which now writes activity.exercise-session.strong. Fact-time rule and
// identity are the earlier version's, which def.Guard holds.
func TestSourceTime_TheWriterlessTypesRequireIt(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	for typ, version := range map[facet.Type]int{
		"activity.gym-visit":      2,
		"health.body-composition": 5,
		"health.heart-rate":       4,
		"health.metric-group":     4,
		"health.sleep":            4,
	} {
		d, ok := reg.Lookup(typ)
		if !ok || d.Version != version {
			t.Errorf("%s = v%d, %v; want v%d", typ, d.Version, ok, version)
			continue
		}
		held := map[int]bool{}
		for _, s := range stored {
			if s.Type != typ {
				continue
			}
			held[s.Version] = true
			if s.Version != version {
				continue
			}
			var m struct {
				Schema struct {
					Required   []string                   `json:"required"`
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"schema"`
			}
			if err := json.Unmarshal(s.Body, &m); err != nil {
				t.Fatal(err)
			}
			if _, ok := m.Schema.Properties[def.SourceTimeField]; !ok {
				t.Errorf("%s v%d does not declare %s", typ, version, def.SourceTimeField)
			}
			if !slices.Contains(m.Schema.Required, def.SourceTimeField) {
				t.Errorf("%s v%d does not require %s", typ, version, def.SourceTimeField)
			}
		}
		if !held[version-1] {
			t.Errorf("%s v%d is not carried in the history", typ, version-1)
		}
	}
}

// Card 198, the records window: the six types read from hand-made records files require
// source_time, each at the version after the one that declared it. Fact-time rule and
// identity are the earlier version's, which def.Guard holds. Weight's carries the offset the
// date had (shape v2), since its fact time is read at it; the others carry shape v1.
func TestSourceTime_TheRecordsFilesTypesRequireIt(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	stored, err := def.Manifests()
	if err != nil {
		t.Fatal(err)
	}
	for typ, version := range map[facet.Type]int{
		"health.weight": 7, "health.lab-result": 3, "health.lab-panel": 3,
		"health.blood-pressure": 8, "health.encounter": 2, "health.donation": 2,
	} {
		d, ok := reg.Lookup(typ)
		if !ok || d.Version != version {
			t.Errorf("%s = v%d, %v; want v%d", typ, d.Version, ok, version)
			continue
		}
		held := map[int]bool{}
		for _, s := range stored {
			if s.Type != typ {
				continue
			}
			held[s.Version] = true
			if s.Version != version {
				continue
			}
			var m struct {
				Schema struct {
					Required []string `json:"required"`
				} `json:"schema"`
			}
			if err := json.Unmarshal(s.Body, &m); err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(m.Schema.Required, def.SourceTimeField) {
				t.Errorf("%s v%d does not require %s", typ, version, def.SourceTimeField)
			}
		}
		if !held[version-1] {
			t.Errorf("%s v%d is not carried in the history", typ, version-1)
		}
	}
}
