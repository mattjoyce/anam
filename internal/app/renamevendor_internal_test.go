package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 195: vendor-shaped types take the vendor leaf. renames/vendor-leaves.json splits
// activity.exercise-session by payload id prefix into a bundled leaf per vendor, and
// renames the three types the importers register to their vendor's leaf, which the
// importers register first. testdata/vendor-leaves holds copies of the importers'
// bodies, old and new, from anam-importers' garmin, withings and dreamstation_card
// definitions folders.

// vendorSessions are one session per vendor, each with the fields its importer or
// extractor writes, in activity.exercise-session v2's shape. The Concept2 and Strong
// sessions say how their clocks were read, as their leaves require from v2 (card
// 198); the live ones crossed at v1, before it, so the table now carries only a
// session that says so.
var vendorSessions = []record{
	{"activity.exercise-session", `{"id": "garmin:session:2026-10-20T09:02:36Z", "activity_name": "walking", "source": "Garmin",
		"effective_time_frame": {"time_interval": {"start_date_time": "2026-10-20T09:02:36Z", "duration": {"value": 1830.2, "unit": "sec"}}},
		"source_time": {"value": "2026-10-20T09:02:36Z", "zone": "UTC", "basis": "source"},
		"fit_sport": "walking", "sport_profile": "Walk", "duration": {"value": 1801, "unit": "sec"}, "kcal_burned": {"value": 140, "unit": "kcal"},
		"distance": {"value": 2412.5, "unit": "m"}, "cumulative_elevation_gain": {"value": 21, "unit": "m"}, "descent_m": 19,
		"avg_hr_bpm": 101, "max_hr_bpm": 122, "training_effect_aerobic": 1.4, "training_load": 12}`},
	{"activity.exercise-session", `{"id": "concept2:session:50445680", "activity_name": "rowing.machine", "source": "Concept2 Logbook",
		"effective_time_frame": {"time_interval": {"start_date_time": "2026-10-04T04:11:37Z", "duration": {"value": 600, "unit": "sec"}}},
		"source_time": {"value": "2026-10-04 04:21:37", "zone": "UTC", "basis": "source"},
		"duration": {"value": 600, "unit": "sec"}, "start_time_known": true, "distance": {"value": 2210, "unit": "m"},
		"average_cadence": {"value": 22, "unit": "strokes/min"}, "drag_factor": 118, "watt_minutes": 1203, "erg_type": "rower",
		"workout_type": "FixedTimeSplits", "logbook_source": "ErgData Android", "logbook_date": "2026-10-04 15:21:37",
		"logbook_timezone": "Australia/Sydney", "weight_class": "H", "verified": true, "ranked": false, "avg_hr_bpm": 131,
		"splits": [{"position": 1, "duration_sec": 300, "distance_m": 1105, "watt_minutes": 601, "avg_spm": 22, "avg_hr_bpm": 128, "min_hr_bpm": 93}],
		"stroke_series": [{"time_sec": 2.5, "distance_m": 6, "pace_500m_sec": 210, "spm": 20}]}`},
	{"activity.exercise-session", `{"id": "technogym:2021-06-20T12:26:46.4054099+10:00", "activity_name": "rowing.machine", "source": "Technogym mywellness",
		"effective_time_frame": {"time_interval": {"start_date_time": "2021-06-20T12:12:48+10:00", "duration": {"value": 838.9, "unit": "sec"}}},
		"duration": {"value": 838.9, "unit": "sec"}, "distance": {"value": 2000, "unit": "m"}, "kcal_burned": {"value": 121, "unit": "kcal"},
		"base_movement_quantity": {"value": 199, "unit": "strokes"}, "average_cadence": {"value": 23.7, "unit": "strokes/min"},
		"avg_power_w": 148, "max_power_w": 205, "max_spm": 27, "split_500m_sec": 126, "level": 6, "move": 420, "ph_id": "ph-1", "facility": "site-1",
		"splits": [{"position": 1, "duration_sec": 252, "distance_m": 1000, "kcal": 62, "strokes": 122, "avg_spm": 24, "avg_power_w": 148, "level": 6}]}`},
	{"activity.exercise-session", `{"id": "strong:session:2020-11-09T06:46:22", "activity_name": "strength_training", "source": "Strong",
		"effective_time_frame": {"time_interval": {"start_date_time": "2020-11-09T06:46:22+11:00", "duration": {"value": 611, "unit": "sec"}}},
		"source_time": {"value": "2020-11-09 06:46:22", "zone": "Australia/Sydney", "basis": "configuration"},
		"workout_name": "Morning Workout", "reported_duration_sec": 611, "exercises": ["Bench Press (Barbell)"],
		"sets": [{"exercise": "Bench Press (Barbell)", "set": "1", "kg": 60, "reps": 8}]}`},
}

// vendorRegistered are the three registered types' facets: a Sydney day, a London day,
// a weight and a therapy session, each in its importer's shape.
var vendorRegistered = []record{
	{"activity.day", `{"id": "day:2026-10-09", "day": "2026-10-09", "step_goal": 5070, "calories_total": 2238,
		"effective_time_frame": {"time_interval": {"start_date_time": "2026-10-09T00:00:00+11:00", "end_date_time": "2026-10-10T00:00:00+11:00"}},
		"source_time": {"value": "2026-10-09", "zone": "+11:00", "basis": "source"}}`},
	{"activity.day", `{"id": "day:2026-10-18", "day": "2026-10-18", "step_goal": 5070, "calories_total": 2105,
		"effective_time_frame": {"time_interval": {"start_date_time": "2026-10-18T00:00:00+01:00", "end_date_time": "2026-10-19T00:00:00+01:00"}},
		"source_time": {"value": "2026-10-18", "zone": "+01:00", "basis": "source"}}`},
	{"health.measurement", `{"withings_id": "3171", "date": "2026-10-14 18:42:38", "type": "1", "value": "81.25", "unit": "kg", "timezone": "Australia/Sydney"}`},
	{"health.cpap-session", `{"device_serial": "P1234", "session": 77, "start": "2026-10-08T12:10:15Z", "duration_seconds": 25200,
		"mask_on_spans": [[0, 25200]], "settings": {"mode": "cpap", "pressure": 9}}`},
}

// vendorLeafOf is the leaf each record arrives at.
func vendorLeafOf(r record) facet.Type {
	if r.typ != "activity.exercise-session" {
		return map[facet.Type]facet.Type{
			"activity.day":        "activity.day.garmin",
			"health.measurement":  "health.measurement.withings",
			"health.cpap-session": "health.cpap-session.dreamstation",
		}[r.typ]
	}
	for prefix, leaf := range map[string]facet.Type{
		`"id": "garmin:session:`:   "activity.exercise-session.garmin",
		`"id": "concept2:session:`: "activity.exercise-session.concept2",
		`"id": "technogym:`:        "activity.exercise-session.technogym",
		`"id": "strong:session:`:   "activity.exercise-session.strong",
	} {
		if strings.Contains(r.payload, prefix) {
			return leaf
		}
	}
	return ""
}

// vendorBodies are the registered types' bodies from testdata, old and new, parsed.
func vendorBodies(t *testing.T) map[facet.Type]def.Definition {
	t.Helper()
	out := map[facet.Type]def.Definition{}
	for _, name := range []string{"activity.day", "health.measurement", "health.cpap-session",
		"activity.day.garmin", "health.measurement.withings", "health.cpap-session.dreamstation"} {
		raw, err := os.ReadFile(filepath.Join("testdata", "vendor-leaves", name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		d, err := def.Parse(raw)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		out[d.Type] = d
	}
	return out
}

// registerVendorBodies registers every body in the fixture's operational store, as the
// importers have registered the old types and register the leaves before the run.
func registerVendorBodies(t *testing.T, f ledgerFixture, bodies map[facet.Type]def.Definition) {
	t.Helper()
	ctx := context.Background()
	buckets, err := config.Config{BucketWidth: span.Default}.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, filepath.Join(f.dir, "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for typ, d := range bodies {
		raw, err := os.ReadFile(filepath.Join("testdata", "vendor-leaves", string(typ)+".json"))
		if err != nil {
			t.Fatal(err)
		}
		row := ops.DefinitionRow{Type: string(typ), Version: d.Version, Body: string(raw)}
		if err := db.RegisterDefinition(ctx, row, "2026-10-21T00:00:00.000000000Z"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRename_TheTableGivesEachVendorItsLeaf(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newLedgerFixture(t)
	bodies := vendorBodies(t)
	held := heldDefs(t, "activity.exercise-session")
	admitting, err := def.NewRegistry(append(held.All(), bodies["activity.day"], bodies["health.measurement"], bodies["health.cpap-session"])...)
	if err != nil {
		t.Fatal(err)
	}
	recs := append(append([]record{}, vendorSessions...), vendorRegistered...)
	before := f.admitHeld(t, admitting, recs)
	registerVendorBodies(t, f, bodies)
	asOf, _, err := cli(t, f.cfgPath, "timeline", "--json", "--w", strconv.FormatInt(int64(before), 10))
	if err != nil {
		t.Fatalf("timeline before: %v", err)
	}

	table := filepath.Join("..", "..", "renames", "vendor-leaves.json")
	out, _, err := cli(t, f.cfgPath, "rename", "--table", table)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	for to, want := range map[facet.Type]string{
		"activity.exercise-session.garmin": "1", "activity.exercise-session.concept2": "1",
		"activity.exercise-session.technogym": "1", "activity.exercise-session.strong": "1",
		"activity.day.garmin": "2", "health.measurement.withings": "1", "health.cpap-session.dreamstation": "1",
	} {
		if row := reportRowTo(t, out, to); row[2] != want || row[3] != "0" {
			t.Errorf("dry run row %v, want %s to carry and 0 refused", row, want)
		}
	}
	// activity.day.garmin reads the interval start (card 197): the London day moves to
	// London midnight, the Sydney day stays where it was.
	if !strings.Contains(out, "activity.day to activity.day.garmin places facets by /effective_time_frame/time_interval/start_date_time") ||
		!strings.Contains(out, "1 of 2 to carry move in fact time") {
		t.Errorf("the dry run does not say the days are re-placed:\n%s", out)
	}
	if strings.Count(out, " places facets by ") != 1 {
		t.Errorf("only activity.day changes its fact-time rule:\n%s", out)
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Fatalf("the dry run wrote: W %d → %d", before, w)
	}

	if out, _, err = cli(t, f.cfgPath, "rename", "--table", table, "--yes"); err != nil {
		t.Fatalf("--yes: %v\n%s", err, out)
	}
	if w := headOf(t, f.ledgerPath); w != before+ledger.Watermark(len(recs)) {
		t.Errorf("W %d, want %d: one event per facet carried", w, before+ledger.Watermark(len(recs)))
	}

	bundled, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Open(ctx, f.ledgerPath, "matt")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range recs {
		leaf := vendorLeafOf(r)
		d, ok := bundled.Lookup(leaf)
		if !ok {
			d, ok = bodies[leaf]
		}
		if !ok {
			t.Fatalf("no Definition for %s", leaf)
		}
		doc, err := def.Decode([]byte(r.payload))
		if err != nil {
			t.Fatal(err)
		}
		ref, err := d.SourceRef("records", doc)
		if err != nil {
			t.Fatal(err)
		}
		cur, err := l.CurrentVersion(ctx, facet.NewID(ref))
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		want, err := facet.Canonicalize([]byte(r.payload))
		if err != nil {
			t.Fatal(err)
		}
		if cur.Type != leaf || cur.ArtifactVersion != 2 || string(cur.Payload.Bytes()) != string(want.Bytes()) {
			t.Errorf("%s is %s v%d, want %s v2 with the payload unchanged", ref, cur.Type, cur.ArtifactVersion, leaf)
		}
		stored, err := def.Decode(cur.Payload.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		if err := d.Check(stored); err != nil {
			t.Errorf("%s does not validate under %s: %v", ref, leaf, err)
		}
	}
	london, err := l.CurrentVersion(ctx, facet.NewID(facet.SourceRef("records", "day:2026-10-18")))
	if err != nil {
		t.Fatal(err)
	}
	if want := time.Date(2026, 10, 17, 23, 0, 0, 0, time.UTC); !london.FactTime.Equal(want) {
		t.Errorf("the London day is at %s, want its London midnight %s", london.FactTime, want)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	// A second dry run finds nothing left to carry, and a read as-of before the run is
	// unchanged.
	if out, _, err = cli(t, f.cfgPath, "rename", "--table", table); err != nil {
		t.Fatalf("second dry run: %v\n%s", err, out)
	}
	for _, r := range recs {
		if row := reportRowTo(t, out, vendorLeafOf(r)); row[2] != "0" || row[3] != "0" {
			t.Errorf("second dry run row %v, want nothing to carry", row)
		}
	}
	if again, _, err := cli(t, f.cfgPath, "timeline", "--json", "--w", strconv.FormatInt(int64(before), 10)); err != nil || again != asOf {
		t.Errorf("timeline at W %d changed across the run (%v):\nbefore %s\nafter  %s", before, err, asOf, again)
	}

	// recent-life v5 serves every exercise leaf by the prefix, and no longer the old name.
	raw, err := os.ReadFile(filepath.Join("..", "..", "projections", "recent-life.v5.json"))
	if err != nil {
		t.Fatal(err)
	}
	tpl, err := projection.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	covered := tpl.Covers(bundled.All())
	for _, leaf := range leaves {
		if !slices.Contains(covered, leaf) {
			t.Errorf("recent-life v5 does not serve %s: %v", leaf, covered)
		}
	}
	var m struct{ Types []string }
	if err := json.Unmarshal(raw, &m); err != nil || slices.Contains(m.Types, "activity.exercise-session") {
		t.Errorf("recent-life v5 still names activity.exercise-session (%v): %v", err, m.Types)
	}
}
