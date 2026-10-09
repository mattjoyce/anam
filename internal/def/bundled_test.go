package def_test

import (
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
		if d.Version != 2 || d.Classification != facet.ClassificationSensitive {
			t.Errorf("%s = v%d %s, want v2 Sensitive", typ, d.Version, d.Classification)
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
