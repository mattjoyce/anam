package def_test

import (
	"testing"

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
		if d.Version != 1 || d.Classification != facet.ClassificationSensitive {
			t.Errorf("%s = v%d %s, want v1 Sensitive", typ, d.Version, d.Classification)
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
		if d.Classification != facet.ClassificationSensitive || d.Mode != def.ModeLenient {
			t.Errorf("%s = %s %s, want Sensitive lenient", typ, d.Classification, d.Mode)
		}
	}
	if !reg.Closes("med.stopped") || reg.Closes("med.started") || reg.Closes("med.dose-changed") {
		t.Error("only med.stopped closes the regimen")
	}
}
