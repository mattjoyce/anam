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

// readsALocalClock are the bundled types whose fact time is a wall clock or a date
// read in the Definition's zone, and which do not yet say how it was read: their
// writers have not moved to source_time (card 197's follow-up). The list only
// shrinks. A type leaves it by requiring source_time, and a new type that reads a
// local clock must require it from its first version.
var readsALocalClock = map[string]bool{
	"activity.gym-visit": true, "health.blood-pressure": true, "health.body-composition": true,
	"health.condition": true, "health.donation": true, "health.encounter": true,
	"health.heart-rate": true, "health.illness.ended": true, "health.illness.started": true,
	"health.lab-panel": true, "health.lab-result": true, "health.medication.compliance": true,
	"health.medication.dose-changed": true,
	"health.medication.dose-missed":  true, "health.medication.started": true,
	"health.medication.stopped": true, "health.metric-group": true, "health.sleep": true,
	"health.vision-prescription": true, "health.weight": true, "place.trip.arrived": true,
	"place.trip.departed": true, "place.trip.returned": true,
}

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
		local := d.FactTime.Zone != "UTC" && !strings.Contains(d.FactTime.Layout, "07") && !strings.Contains(d.FactTime.Layout, "Z")
		listed := readsALocalClock[string(d.Type)]
		switch {
		case local && !required[string(d.Type)] && !listed:
			t.Errorf("%s reads %q in %s and does not require %s", d.Type, d.FactTime.Layout, d.FactTime.Zone, def.SourceTimeField)
		case listed && (required[string(d.Type)] || !local):
			t.Errorf("%s is listed as not yet saying how its clock was read, and it does: take it off the list", d.Type)
		}
	}
	for typ := range readsALocalClock {
		if _, ok := reg.Lookup(facet.Type(typ)); !ok {
			t.Errorf("%s is listed and not bundled: take it off the list", typ)
		}
	}
}

// Card 198: the importers' types say how their clocks were read. A type whose every
// writer sends source_time requires it; one also written by hand into a records file,
// or whose source has no export left to extract again, declares it without requiring
// it, so its old payloads still admit.
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
		"health.blood-pressure":               {7, false},
		"health.lab-result":                   {2, false},
		"health.lab-panel":                    {2, false},
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
