package inspect

import (
	"reflect"
	"testing"
)

// These literals pin the personal grammar before card 256 removes it from the
// engine. Base d4d15306ffd64d0e2548c57b4471fdf067325c90 has the same inspector
// sources as 9a3b532ac8452cdc1e75640993ba754e9928a6d2. Do not regenerate these
// expectations from a presentation fixture or from the implementation.
type parityDomain struct {
	ID  string
	Hue int
}

type parityFamily struct{ ID, Label string }
type parityOverride struct{ Type, Family, Kind, Shape string }
type parityGrammar struct {
	Domains   []parityDomain
	Families  []parityFamily
	Overrides map[string]parityOverride
}

func pinnedParityGrammar() parityGrammar {
	return parityGrammar{
		Domains: []parityDomain{
			{"health", 7}, {"activity", 3}, {"place", 1}, {"record", 0},
			{"mind", 6}, {"org", 5}, {"money", 2},
			{"social", -1}, {"media", -1}, {"work", -1},
		},
		Families: []parityFamily{
			{"vitals", "vitals"}, {"body", "body"}, {"sleep", "sleep"},
			{"movement", "movement"}, {"exercise", "exercise"}, {"labs", "labs"},
			{"conditions", "conditions"}, {"meds", "meds"}, {"intake", "intake"},
			{"clinical", "clinical"}, {"location", "location"}, {"trips", "trips"},
			{"attention", "attention"}, {"employment", "employment"},
			{"purchases", "purchases"}, {"notes", "notes"}, {"documents", "documents"},
		},
		Overrides: map[string]parityOverride{
			"health.heart-rate":             {"health.heart-rate", "vitals", "measure", "circle"},
			"health.sleep":                  {"health.sleep", "sleep", "measure", "circle"},
			"health.metric-group":           {"health.metric-group", "movement", "measure", "circle"},
			"health.blood-pressure":         {"health.blood-pressure", "vitals", "reading", "square"},
			"health.heart-rate-alert":       {"health.heart-rate-alert", "vitals", "event", "alert"},
			"health.weight":                 {"health.weight", "body", "reading", "square"},
			"health.measurement.withings":   {"health.measurement.withings", "body", "reading", "diamond"},
			"activity.gym-visit":            {"activity.gym-visit", "exercise", "event", "square"},
			"health.lab-panel":              {"health.lab-panel", "labs", "event", "square"},
			"health.medication.started":     {"health.medication.started", "meds", "event", "tri-up"},
			"health.medication.stopped":     {"health.medication.stopped", "meds", "event", "tri-down"},
			"health.medication.dose-missed": {"health.medication.dose-missed", "meds", "event", "cross"},
			"health.medication.dose":        {"health.medication.dose", "meds", "event", "square"},
			"health.illness.started":        {"health.illness.started", "conditions", "event", "tri-up"},
			"health.illness.ended":          {"health.illness.ended", "conditions", "event", "tri-down"},
			"mind.attention.started":        {"mind.attention.started", "attention", "event", "tri-up"},
			"mind.attention.ended":          {"mind.attention.ended", "attention", "event", "tri-down"},
			"org.employment.started":        {"org.employment.started", "employment", "event", "tri-up"},
			"org.employment.role-changed":   {"org.employment.role-changed", "employment", "event", "diamond"},
			"org.employment.ended":          {"org.employment.ended", "employment", "event", "tri-down"},
			"money.purchase":                {"money.purchase", "purchases", "event", "diamond"},
			"health.encounter":              {"health.encounter", "clinical", "event", "square"},
			"health.donation":               {"health.donation", "clinical", "event", "star"},
			"place.trip.departed":           {"place.trip.departed", "trips", "event", "tri-up"},
			"place.trip.arrived":            {"place.trip.arrived", "trips", "event", "tri-down"},
			"place.trip.flight":             {"place.trip.flight", "trips", "event", "cross"},
			"record.document":               {"record.document", "documents", "event", "square"},
			"record.link":                   {"record.link", "documents", "event", "cross"},
		},
	}
}

func TestParityGrammar(t *testing.T) {
	t.Parallel()
	got, want := currentParityGrammar(), pinnedParityGrammar()
	if !reflect.DeepEqual(got.Domains, want.Domains) {
		t.Errorf("domain order/hue changed\ngot  %v\nwant %v", got.Domains, want.Domains)
	}
	if !reflect.DeepEqual(got.Families, want.Families) {
		t.Errorf("family order/label changed\ngot  %v\nwant %v", got.Families, want.Families)
	}
	if len(got.Overrides) != len(want.Overrides) {
		t.Errorf("override count changed, got %d want 28", len(got.Overrides))
	}
	for typ, w := range want.Overrides {
		if g := got.Overrides[typ]; g != w {
			t.Errorf("%s override changed, got %+v want %+v", typ, g, w)
		}
	}
}
