package def_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// The bundled set still holds the retired health-db source's types (card 55): every
// tombstoned facet of those types must still resolve for the fold and the inspector.
func TestBundled_HoldsTheHealthDBTypes(t *testing.T) {
	t.Parallel()

	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}

	// bp-csv's type keys on (date_excel, reading) — the export carries no reading
	// id and distinct readings share timestamps, so the ordinal is identity.
	bp, ok := reg.Lookup("health.blood-pressure")
	if !ok {
		t.Error("no bundled definition for health.blood-pressure")
	} else if len(bp.IdentityKey) != 2 || bp.IdentityKey[1] != "/reading" {
		t.Errorf("health.blood-pressure identity_key = %v, want [/date_excel /reading]", bp.IdentityKey)
	}

	// The three retired at card 55 are left as written but for source_time (card 198), at
	// v4; body composition took kind and family at v4 (card 147) and source_time at v5.
	for typ, version := range map[facet.Type]int{
		"health.sleep": 4, "health.body-composition": 5, "health.heart-rate": 4, "health.metric-group": 4,
	} {
		d, ok := reg.Lookup(typ)
		if !ok {
			t.Errorf("no bundled definition for %s", typ)
			continue
		}
		if d.Version != version || d.Classification != facet.ClassificationSensitive {
			t.Errorf("%s = v%d %s, want v%d Sensitive", typ, d.Version, d.Classification, version)
		}
		// All four share the identity grain: one facet per (group, period_type,
		// period_start). The worker composes source_ref on the same promise.
		if len(d.IdentityKey) != 3 || d.IdentityKey[0] != "/group" {
			t.Errorf("%s identity_key = %v, want [/group /period_type /period_start]", typ, d.IdentityKey)
		}
	}
}

// The regimen types share one series family with one key, and only the stop closes.
func TestBundled_MedTypesShareTheRegimenSeries(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	for _, typ := range []facet.Type{"health.medication.started", "health.medication.dose-changed", "health.medication.stopped"} {
		d, ok := reg.Lookup(typ)
		if !ok {
			t.Errorf("no bundled definition for %s", typ)
			continue
		}
		if d.Series == nil || d.Series.Name != "med.regimen" || len(d.Series.Key) != 2 {
			t.Errorf("%s series = %+v, want med.regimen over [/drug /course]", typ, d.Series)
		}
		if d.Classification != facet.ClassificationSensitive || d.Mode != def.ModeStrict {
			t.Errorf("%s = %s %s, want Sensitive strict", typ, d.Classification, d.Mode)
		}
	}
	if !reg.Closes("health.medication.stopped") || reg.Closes("health.medication.started") || reg.Closes("health.medication.dose-changed") {
		t.Error("only health.medication.stopped closes the regimen")
	}
}

// The life-event types: illness and travel are series, closed by
// their own ending type; donation and gym stand alone. Medical ones are
// Sensitive, travel and gym Confidential.
func TestBundled_LifeEventTypes(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	cases := []struct {
		typ    facet.Type
		class  facet.Classification
		series string
		closes bool
	}{
		{"health.illness.started", facet.ClassificationSensitive, "illness.episode", false},
		{"health.illness.ended", facet.ClassificationSensitive, "illness.episode", true},
		{"place.trip.departed", facet.ClassificationConfidential, "travel.trip", false},
		{"place.trip.arrived", facet.ClassificationConfidential, "travel.trip", false},
		{"place.trip.returned", facet.ClassificationConfidential, "travel.trip", true},
		{"place.trip.flight", facet.ClassificationConfidential, "", false},
		{"health.donation", facet.ClassificationSensitive, "", false},
		{"activity.gym-visit", facet.ClassificationConfidential, "", false},
	}
	for _, c := range cases {
		d, ok := reg.Lookup(c.typ)
		if !ok {
			t.Errorf("no bundled definition for %s", c.typ)
			continue
		}
		if d.Classification != c.class || d.Mode != def.ModeStrict {
			t.Errorf("%s = %s %s, want %s strict", c.typ, d.Classification, d.Mode, c.class)
		}
		switch {
		case c.series == "" && d.Series != nil:
			t.Errorf("%s declares a series %q, want none", c.typ, d.Series.Name)
		case c.series != "" && (d.Series == nil || d.Series.Name != c.series):
			t.Errorf("%s series = %+v, want %s", c.typ, d.Series, c.series)
		}
		if reg.Closes(c.typ) != c.closes {
			t.Errorf("%s closes = %t, want %t", c.typ, reg.Closes(c.typ), c.closes)
		}
	}
}

// A flight is evidence of a movement, not a state of the trip, so it stands outside
// the travel.trip series and only carries the trip's key. Inside the series a flight
// dated in Singapore would sort after a return dated the same day in Sydney and hold
// the trip open for good.
//
// A flight is dated where it left from. Its date carries the departure airport's own
// offset, because a trip's legs are dated in different zones and one zone per type
// would put a London departure up to a day from where it happened.
func TestBundled_FlightIsDatedWhereItLeft(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	d, ok := reg.Lookup("place.trip.flight")
	if !ok {
		t.Fatal("no bundled definition for place.trip.flight")
	}
	cases := []struct{ date, want string }{
		{"2018-10-10 +01:00", "2018-10-09T23:00:00Z"}, // SQ317 out of London, BST
		{"2024-12-27 +11:00", "2024-12-26T13:00:00Z"}, // SQ222 out of Sydney, AEDT
	}
	for _, c := range cases {
		doc, err := def.Decode([]byte(`{"id":"x","trip":"t","from":"LHR","to":"SIN","date":"` + c.date + `"}`))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if err := d.Check(doc); err != nil {
			t.Errorf("%s: %v", c.date, err)
		}
		got, err := d.FactTimeOf(doc)
		if err != nil {
			t.Fatalf("%s: %v", c.date, err)
		}
		if g := got.UTC().Format(time.RFC3339); g != c.want {
			t.Errorf("%s placed at %s, want %s", c.date, g, c.want)
		}
	}
}

// record.document is the first reference-kind type: the descriptor says the payload is
// a pointer, and the Definition requires what the pointer must say.
func TestBundled_DocReferenceIsAReference(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	d, ok := reg.Lookup("record.document")
	if !ok {
		t.Fatal("no bundled definition for record.document")
	}
	if d.PayloadKind != facet.PayloadReference || len(d.IdentityKey) != 1 || d.IdentityKey[0] != "/path" {
		t.Errorf("record.document = kind %s identity %v", d.PayloadKind, d.IdentityKey)
	}
	bp, _ := reg.Lookup("health.blood-pressure")
	if bp.PayloadKind != facet.PayloadVerbatim {
		t.Errorf("a type that says nothing is verbatim, got %s", bp.PayloadKind)
	}
}

// Identify composes the ref the gate admits under, from the payload alone: a ref the
// envelope carries does not change the answer, and a type with no Definition has none.
func TestRegistry_Identify(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	payload := []byte(`{"path":"scans/bp.jpg","sha256":"00","media_type":"image/jpeg","bytes":10,"date":"2024-09-16"}`)
	want := facet.SourceRef("documents", "scans/bp.jpg")
	for _, carried := range []string{"", want, "documents:something-else"} {
		env := facet.Envelope{Type: "record.document", Source: "documents", SourceRef: carried, Payload: payload}
		if got, err := reg.Identify(env); err != nil || got != want {
			t.Errorf("carrying %q: Identify = %q, %v; want %q", carried, got, err, want)
		}
	}
	if _, err := reg.Identify(facet.Envelope{Type: "no.such", Source: "documents", Payload: payload}); err == nil {
		t.Error("Identify composed a ref for a type with no Definition")
	}
}

// The lab extraction script writes time_unknown on a draw whose report printed no
// time. The bundled health.lab-result must admit what its own extractor writes.
func TestBundled_LabResultAdmitsAnUntimedDraw(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	d, ok := reg.Lookup("health.lab-result")
	if !ok {
		t.Fatal("no bundled health.lab-result")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"id": "acl:12345678/potassium", "collected": "2026-03-04 00:00",
		"source_time": {"value": "2026-03-04 00:00", "zone": "Australia/Sydney", "basis": "source"},
		"analyte": "Potassium", "value": "4.6", "lab_number": "12345678", "time_unknown": true}`), &payload); err != nil {
		t.Fatal(err)
	}
	if err := d.Check(payload); err != nil {
		t.Errorf("an untimed draw was refused: %v", err)
	}
	// The field is declared, not ignored: a value of the wrong type is still refused.
	payload["time_unknown"] = "yes"
	if err := d.Check(payload); err == nil {
		t.Error("time_unknown as a string was admitted")
	}
}

// Card 83: blood pressure v4 is the Open mHealth blood-pressure-4.0 body with the
// source's own fields beside it. Its identity and fact time are v3's, so the guard
// passes and the readings supersede in place; a v3-shaped payload is refused. Card
// 97: v5 names the six export columns v4 refused, as text, so one ticked checkbox
// no longer stops the export. Card 147: v6 is v5 declaring kind and family. Card
// 198: v7 declares source_time, v8 requires it. The bundled file is byte for byte the
// importer's.
func TestBundled_BloodPressureIsAnOMHBody(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup("health.blood-pressure")
	if !ok {
		t.Fatal("no bundled health.blood-pressure")
	}
	if d.Version != 8 || d.FactTime.Pointer != "/date_excel" || len(d.IdentityKey) != 2 || d.Kind != def.KindReading || d.Family != "vitals" {
		t.Fatalf("v%d fact time %s identity %v", d.Version, d.FactTime.Pointer, d.IdentityKey)
	}
	decode := func(s string) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	v4 := `{"date_excel": "2026-1-24 06:30:00", "reading": "1",
		"source_time": {"value": "2026-1-24 06:30:00", "zone": "Australia/Sydney", "basis": "source"},
		"systolic_blood_pressure": {"value": 128.0, "unit": "mmHg"},
		"diastolic_blood_pressure": {"value": 79.0, "unit": "mmHg"},
		"effective_time_frame": {"date_time": "2026-01-24T06:30:00+11:00"},
		"body_posture": "sitting", "pulse": "64.0", "location": "Left arm"}`
	if err := d.Check(decode(v4)); err != nil {
		t.Errorf("a v4 payload was refused: %v", err)
	}
	v5 := strings.Replace(v4, `"location": "Left arm"}`, `"location": "Left arm",
		"irregular_heartbeat": "Yes", "dizzy_light_headed": "Yes", "temperature_celsius": "36.6",
		"blood_glucose_mg_dl": "95", "oxygen_saturation_pct": "98", "tags": "morning;fasting"}`, 1)
	if err := d.Check(decode(v5)); err != nil {
		t.Errorf("a v5 payload with every export column was refused: %v", err)
	}
	for name, payload := range map[string]string{
		"v3 shape":       `{"date_excel": "2026-1-24 06:30:00", "reading": "1", "systolic": "128.0", "diastolic": "79.0", "source_time": {"value": "2026-1-24 06:30:00", "zone": "Australia/Sydney", "basis": "source"}}`,
		"no source_time": strings.Replace(v4, `"source_time": {"value": "2026-1-24 06:30:00", "zone": "Australia/Sydney", "basis": "source"},`, "", 1),
		"kPa":            strings.Replace(v4, `"value": 128.0, "unit": "mmHg"`, `"value": 17.1, "unit": "kPa"`, 1),
		"no frame":       strings.Replace(v4, `"effective_time_frame": {"date_time": "2026-01-24T06:30:00+11:00"},`, "", 1),
		"a posture":      strings.Replace(v4, `"sitting"`, `"Seated"`, 1),
	} {
		if err := d.Check(decode(payload)); err == nil {
			t.Errorf("%s was admitted", name)
		}
	}
}

// A note is the person's words: it names the facets it is about by id, when there
// are any, and stands on the timeline at the moment it was written. The facets it
// names are never touched by it. Since v3 (card 157) a note may name none, since an
// account can come before what it describes; an empty list, or a name that is not a
// facet id, is still refused.
func TestBundled_NoteAnnotationIsThePersonsWords(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup("record.edge")
	if !ok {
		t.Fatal("no bundled record.edge")
	}
	if d.Version != 3 || d.FactTime.Pointer != "/at" || len(d.IdentityKey) != 1 || d.IdentityKey[0] != "/id" {
		t.Fatalf("v%d fact time %s identity %v", d.Version, d.FactTime.Pointer, d.IdentityKey)
	}
	decode := func(s string) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	// v2 of record.edge (card 198) requires source_time: the zone the note was said in.
	note := `{"id": "note:2c6f38a3:1", "at": "2026-10-09 11:29",
		"source_time": {"value": "2026-10-09 11:29", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"},
		"about": ["2c6f38a3-3db7-5e65-b933-9c59980e1c6c", "fe61ec2a-6d8b-56b0-8930-e3270f4ad44c"],
		"text": "During a large work meeting, presenting."}`
	if err := d.Check(decode(note)); err != nil {
		t.Errorf("a note was refused: %v", err)
	}
	unbound := strings.Replace(note, `"about": ["2c6f38a3-3db7-5e65-b933-9c59980e1c6c", "fe61ec2a-6d8b-56b0-8930-e3270f4ad44c"],`, "", 1)
	if err := d.Check(decode(unbound)); err != nil {
		t.Errorf("a note that names no facet was refused: %v", err)
	}
	for name, payload := range map[string]string{
		"about nothing":          strings.Replace(note, `["2c6f38a3-3db7-5e65-b933-9c59980e1c6c", "fe61ec2a-6d8b-56b0-8930-e3270f4ad44c"]`, `[]`, 1),
		"not a facet id":         strings.Replace(note, `2c6f38a3-3db7-5e65-b933-9c59980e1c6c`, `the heart rate alert`, 1),
		"the same id twice":      strings.Replace(note, `fe61ec2a-6d8b-56b0-8930-e3270f4ad44c`, `2c6f38a3-3db7-5e65-b933-9c59980e1c6c`, 1),
		"empty text":             strings.Replace(note, `"During a large work meeting, presenting."`, `""`, 1),
		"a field of its own":     strings.Replace(note, `"text":`, `"mood": "tense", "text":`, 1),
		"about and to both":      strings.Replace(note, `"about":`, `"to": ["7d1b2c3e-4f5a-5b6c-8d7e-9f0a1b2c3d4e"], "about":`, 1),
		"a kind of its own":      strings.Replace(note, `"text":`, `"kind": "caused_by", "text":`, 1),
		"neither words nor ends": `{"id": "note:2c6f38a3:9", "at": "2026-10-09 11:29"}`,
		"a pin that is no hash":  strings.Replace(note, `"text":`, `"pins": {"2c6f38a3-3db7-5e65-b933-9c59980e1c6c": "3f1c"}, "text":`, 1),
	} {
		if err := d.Check(decode(payload)); err == nil {
			t.Errorf("%s was admitted", name)
		}
	}
	// v2 (card 104): the note is an edge. A v2 writer sends `to`; an edge may join
	// facets with no words, name its from side, say its kind, and pin a version.
	for name, payload := range map[string]string{
		"to, the v2 name": strings.Replace(note, `"about":`, `"to":`, 1),
		"an edge, no words": `{"id": "interp:1", "at": "2026-10-09 11:29", "source_time": {"value": "2026-10-09 11:29", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}, "kind": "interpreted_from",
			"from": ["7d1b2c3e-4f5a-5b6c-8d7e-9f0a1b2c3d4e"], "to": ["2c6f38a3-3db7-5e65-b933-9c59980e1c6c"],
			"pins": {"2c6f38a3-3db7-5e65-b933-9c59980e1c6c": "sha256:3f1c9a0e7b2d4c6f8a1e5b9d0c3f7a2e6b4d8c1f9e0a3b5d7c2f4e6a8b1d3c5e"}}`,
		"kind said": strings.Replace(note, `"text":`, `"kind": "about", "text":`, 1),
	} {
		if err := d.Check(decode(payload)); err != nil {
			t.Errorf("%s was refused: %v", name, err)
		}
	}
}

// Card 36: a lens prescription as an optometrist issued it, each eye's refraction
// as printed. The row is the 2023 prescription in the records file, with the
// provider's details made generic.
func TestBundledVisionPrescription(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup("health.vision-prescription")
	if !ok {
		t.Fatal("no bundled health.vision-prescription")
	}
	if d.Version != 1 || d.FactTime.Pointer != "/date" || d.FactTime.Layout != "2006-01-02" ||
		len(d.IdentityKey) != 1 || d.IdentityKey[0] != "/id" || d.Series != nil {
		t.Fatalf("v%d fact time %s %s identity %v series %v", d.Version, d.FactTime.Pointer, d.FactTime.Layout, d.IdentityKey, d.Series)
	}
	decode := func(s string) map[string]any {
		t.Helper()
		var m map[string]any
		if err := json.Unmarshal([]byte(s), &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	row := `{"id": "example-optical:2023-01-06", "date": "2023-01-06", "expires": "2024-01-06",
		"provider": "Example Optical", "practitioner": "A. Example, provider number 000000AA", "kind": "spectacles",
		"right": {"sphere": 3.25, "cylinder": -1, "axis": 90, "intermediate_add": 1.5, "add": 2},
		"left": {"sphere": 1.5, "cylinder": -1, "axis": 90, "intermediate_add": 1.5, "add": 2},
		"source_note": "read from the transcription of the prescription attached to the 6 Jan 2023 email"}`
	if err := d.Check(decode(row)); err != nil {
		t.Errorf("the prescription was refused: %v", err)
	}
	sphereOnly := `{"id": "example-optical:2023-01-06", "date": "2023-01-06", "provider": "Example Optical",
		"kind": "spectacles", "right": {"sphere": 3.25}, "left": {"sphere": 1.5}}`
	if err := d.Check(decode(sphereOnly)); err != nil {
		t.Errorf("a prescription with only spheres was refused: %v", err)
	}
	for name, payload := range map[string]string{
		"no kind":           strings.Replace(row, `"kind": "spectacles",`, ``, 1),
		"a kind of its own": strings.Replace(row, `"spectacles"`, `"sunglasses"`, 1),
		"an axis past 180": strings.Replace(row, `"axis": 90, "intermediate_add": 1.5, "add": 2},
		"left"`, `"axis": 190, "intermediate_add": 1.5, "add": 2},
		"left"`, 1),
		"a sphere as text":   strings.Replace(row, `"sphere": 3.25`, `"sphere": "+3.25"`, 1),
		"an eye without one": strings.Replace(row, `"left": {"sphere": 1.5, `, `"left": {`, 1),
		"a field of its own": strings.Replace(row, `"kind":`, `"pupillary_distance": 64, "kind":`, 1),
		"a date with a time": strings.Replace(row, `"date": "2023-01-06"`, `"date": "2023-01-06 14:40"`, 1),
	} {
		if err := d.Check(decode(payload)); err == nil {
			t.Errorf("%s was admitted", name)
		}
	}
}

// A Definition's words are read by people and agents as what the type is. They say
// the thing itself and never which kanban card made it; the frozen history keeps the
// words as they were.
func TestBundled_WordsNameNoCard(t *testing.T) {
	t.Parallel()
	cardRef := regexp.MustCompile(`(?i)\bcards?[ -]\d`)
	for _, dir := range []string{"manifests", "sourcetime"} {
		files, err := filepath.Glob(filepath.Join(dir, "*.json"))
		if err != nil || len(files) == 0 {
			t.Fatalf("glob %s: %v (%d files)", dir, err, len(files))
		}
		for _, f := range files {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			if loc := cardRef.FindIndex(raw); loc != nil {
				t.Errorf("%s names a card: %q", f, raw[max(0, loc[0]-40):loc[1]+20])
			}
		}
	}
}
