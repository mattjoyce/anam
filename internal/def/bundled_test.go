package def_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// The bundled set is the MVP's health-db contract surface: every type the capture
// worker can emit must resolve, or ingest dies at the first envelope of that type.
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

	for _, typ := range []facet.Type{
		"health.sleep", "health.body-composition", "health.heart-rate", "health.metric-group",
	} {
		d, ok := reg.Lookup(typ)
		if !ok {
			t.Errorf("no bundled definition for %s", typ)
			continue
		}
		if d.Version != 3 || d.Classification != facet.ClassificationSensitive {
			t.Errorf("%s = v%d %s, want v3 Sensitive", typ, d.Version, d.Classification)
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
	for _, typ := range []facet.Type{"med.started", "med.dose-changed", "med.stopped"} {
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
	if !reg.Closes("med.stopped") || reg.Closes("med.started") || reg.Closes("med.dose-changed") {
		t.Error("only med.stopped closes the regimen")
	}
}

// The life-event types (SELF-LOG.md §4): illness and travel are series, closed by
// their own ending type; hangover, donation and gym stand alone. Medical ones are
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
		{"illness.started", facet.ClassificationSensitive, "illness.episode", false},
		{"illness.ended", facet.ClassificationSensitive, "illness.episode", true},
		{"illness.hangover", facet.ClassificationSensitive, "", false},
		{"travel.departed", facet.ClassificationConfidential, "travel.trip", false},
		{"travel.arrived", facet.ClassificationConfidential, "travel.trip", false},
		{"travel.returned", facet.ClassificationConfidential, "travel.trip", true},
		{"travel.flight", facet.ClassificationConfidential, "", false},
		{"donation.blood", facet.ClassificationSensitive, "", false},
		{"gym.visit", facet.ClassificationConfidential, "", false},
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
	d, ok := reg.Lookup("travel.flight")
	if !ok {
		t.Fatal("no bundled definition for travel.flight")
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

// doc.reference is the first reference-kind type: the descriptor says the payload is
// a pointer, and the Definition requires what the pointer must say.
func TestBundled_DocReferenceIsAReference(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("Bundled: %v", err)
	}
	d, ok := reg.Lookup("doc.reference")
	if !ok {
		t.Fatal("no bundled definition for doc.reference")
	}
	if d.PayloadKind != facet.PayloadReference || len(d.IdentityKey) != 1 || d.IdentityKey[0] != "/path" {
		t.Errorf("doc.reference = kind %s identity %v", d.PayloadKind, d.IdentityKey)
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
		env := facet.Envelope{Type: "doc.reference", Source: "documents", SourceRef: carried, Payload: payload}
		if got, err := reg.Identify(env); err != nil || got != want {
			t.Errorf("carrying %q: Identify = %q, %v; want %q", carried, got, err, want)
		}
	}
	if _, err := reg.Identify(facet.Envelope{Type: "no.such", Source: "documents", Payload: payload}); err == nil {
		t.Error("Identify composed a ref for a type with no Definition")
	}
}

// The lab extraction script writes time_unknown on a draw whose report printed no
// time. The bundled lab.result must admit what its own extractor writes.
func TestBundled_LabResultAdmitsAnUntimedDraw(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	d, ok := reg.Lookup("lab.result")
	if !ok {
		t.Fatal("no bundled lab.result")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(`{"id": "acl:12345678/potassium", "collected": "2026-03-04 00:00",
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
// no longer stops the export. The bundled file is byte for byte the importer's.
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
	if d.Version != 5 || d.FactTime.Pointer != "/date_excel" || len(d.IdentityKey) != 2 {
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
		"v3 shape":  `{"date_excel": "2026-1-24 06:30:00", "reading": "1", "systolic": "128.0", "diastolic": "79.0"}`,
		"kPa":       strings.Replace(v4, `"value": 128.0, "unit": "mmHg"`, `"value": 17.1, "unit": "kPa"`, 1),
		"no frame":  strings.Replace(v4, `"effective_time_frame": {"date_time": "2026-01-24T06:30:00+11:00"},`, "", 1),
		"a posture": strings.Replace(v4, `"sitting"`, `"Seated"`, 1),
	} {
		if err := d.Check(decode(payload)); err == nil {
			t.Errorf("%s was admitted", name)
		}
	}
}

// A note is a fact about facts: it names the facets it is about by id, one or more,
// carries the person's words, and stands on the timeline at the moment it is about.
// The facets it names are never touched by it. A note that names no facet, or one
// that is not a facet id, is refused.
func TestBundled_NoteAnnotationIsAFactAboutAFacet(t *testing.T) {
	t.Parallel()
	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	d, ok := reg.Lookup("note.annotation")
	if !ok {
		t.Fatal("no bundled note.annotation")
	}
	if d.Version != 1 || d.FactTime.Pointer != "/at" || len(d.IdentityKey) != 1 || d.IdentityKey[0] != "/id" {
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
	note := `{"id": "note:2c6f38a3:1", "at": "2026-10-09 11:29",
		"about": ["2c6f38a3-3db7-5e65-b933-9c59980e1c6c", "fe61ec2a-6d8b-56b0-8930-e3270f4ad44c"],
		"text": "During a large work meeting, presenting."}`
	if err := d.Check(decode(note)); err != nil {
		t.Errorf("a note was refused: %v", err)
	}
	for name, payload := range map[string]string{
		"no about":           strings.Replace(note, `"about": "2c6f38a3-3db7-5e65-b933-9c59980e1c6c",`, "", 1),
		"not a facet id":     strings.Replace(note, `2c6f38a3-3db7-5e65-b933-9c59980e1c6c`, `the heart rate alert`, 1),
		"empty text":         strings.Replace(note, `"During a large work meeting, presenting."`, `""`, 1),
		"a field of its own": strings.Replace(note, `"text":`, `"mood": "tense", "text":`, 1),
	} {
		if err := d.Check(decode(payload)); err == nil {
			t.Errorf("%s was admitted", name)
		}
	}
}
