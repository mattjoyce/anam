package def_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// Card 205: one day's resting, minimum and maximum heart rate are one fact. The day
// is the identity, the interval Garmin counted places it, and a figure the source
// left out is a field the payload does not carry. health.heart-rate-day, which held
// them as three facts, stays bundled for the facts admitted under it.
func TestBundled_HeartRateDayIsOneFactADay(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup("health.heart-rate.day")
	if !ok {
		t.Fatal("no bundled health.heart-rate.day")
	}
	if d.Version != 1 || d.Kind != def.KindMeasure || d.Family != "vitals" ||
		d.Classification != facet.ClassificationSensitive || len(d.IdentityKey) != 1 || d.IdentityKey[0] != "/id" ||
		d.FactTime.Pointer != "/effective_time_frame/time_interval/start_date_time" {
		t.Fatalf("v%d %s %s %s identity %v fact time %s", d.Version, d.Kind, d.Family, d.Classification,
			d.IdentityKey, d.FactTime.Pointer)
	}
	if _, ok := reg.Lookup("health.heart-rate-day"); !ok {
		t.Error("health.heart-rate-day is no longer bundled; its admitted facts would not resolve")
	}

	decode := func(s string) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	// The live 17 February 2026, Garmin's interval for it and its three figures.
	full := decode(`{"id": "2026-02-17",
		"resting": {"value": 57, "unit": "beats/min"},
		"minimum": {"value": 56, "unit": "beats/min"},
		"maximum": {"value": 113, "unit": "beats/min"},
		"effective_time_frame": {"time_interval": {"start_date_time": "2026-02-17T00:00:00+11:00",
			"end_date_time": "2026-02-17T22:18:00+11:00"}},
		"source_time": {"value": "2026-02-17", "zone": "+11:00", "basis": "source"}}`)
	// The live 18 February 2026: a resting figure only, and no summary from Garmin, so
	// the day is assumed midnight to midnight in Sydney and says so.
	resting := decode(`{"id": "2026-02-18",
		"resting": {"value": 59, "unit": "beats/min"},
		"effective_time_frame": {"time_interval": {"start_date_time": "2026-02-18T00:00:00+11:00",
			"end_date_time": "2026-02-19T00:00:00+11:00"}},
		"source_time": {"value": "2026-02-18", "zone": "Australia/Sydney", "basis": "assumption"}}`)

	for name, tc := range map[string]struct {
		payload map[string]any
		at      time.Time
	}{
		"all three figures": {full, time.Date(2026, 2, 16, 13, 0, 0, 0, time.UTC)},
		"a figure missing":  {resting, time.Date(2026, 2, 17, 13, 0, 0, 0, time.UTC)},
	} {
		if err := d.Check(tc.payload); err != nil {
			t.Errorf("%s was refused: %v", name, err)
			continue
		}
		at, err := d.FactTimeOf(tc.payload)
		if err != nil || !at.Equal(tc.at) {
			t.Errorf("%s: fact time %v, %v; want the interval start %v", name, at, err, tc.at)
		}
	}

	// The day is the identity: the figures are what it holds, not which fact it is.
	a, errA := d.SourceRef("garmin", full)
	b, errB := d.SourceRef("garmin", decode(`{"id": "2026-02-17", "maximum": {"value": 120, "unit": "beats/min"}}`))
	if errA != nil || errB != nil || a != b || a != facet.SourceRef("garmin", "2026-02-17") {
		t.Errorf("identity %q (%v) and %q (%v), want both the day's", a, errA, b, errB)
	}

	// edit is the full day with keys dropped and fields set.
	edit := func(set map[string]any, drop ...string) map[string]any {
		out := map[string]any{}
		for k, v := range full {
			out[k] = v
		}
		for _, k := range drop {
			delete(out, k)
		}
		for k, v := range set {
			out[k] = v
		}
		return out
	}
	refused := map[string]map[string]any{
		"no source_time":    edit(nil, "source_time"),
		"no frame":          edit(nil, "effective_time_frame"),
		"no figure":         edit(nil, "resting", "minimum", "maximum"),
		"the old statistic": edit(map[string]any{"statistic": "resting"}),
		"another unit":      edit(map[string]any{"resting": map[string]any{"value": 1.0, "unit": "Hz"}}),
		"a bare number":     edit(map[string]any{"maximum": 113.0}),
	}
	for name, payload := range refused {
		if err := d.Check(payload); err == nil {
			t.Errorf("%s was admitted", name)
		}
	}
}
