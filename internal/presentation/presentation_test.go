package presentation_test

import (
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/presentation"
)

func TestCompileIsolation(t *testing.T) {
	t.Parallel()
	hue := 0
	spec := presentation.Spec{
		Domains:  []presentation.DomainSpec{{ID: "custom", Hue: &hue}},
		Families: []presentation.Family{{ID: "1--family", Label: " <Label> "}},
		Types:    []presentation.Override{{Type: "custom.arrival", Shape: "star"}},
	}
	p, err := presentation.Compile(spec)
	if err != nil {
		t.Fatal(err)
	}
	wantD, wantF, wantT := p.Domains(), p.Families(), p.Overrides()
	hue = 7
	spec.Domains[0].ID = "changed"
	spec.Families[0].Label = "changed"
	spec.Types[0].Shape = "cross"
	p.Domains()[0].Hue = 6
	p.Families()[0].Label = "changed"
	p.Overrides()[0].Shape = "cross"
	o, _ := p.Lookup("custom.arrival")
	o.Shape = "square"
	if !reflect.DeepEqual(p.Domains(), wantD) || !reflect.DeepEqual(p.Families(), wantF) || !reflect.DeepEqual(p.Overrides(), wantT) || p.Hue("custom") != "h0" {
		t.Fatal("caller mutated compiled presentation")
	}
	if p.Hue("unconfigured") != "hx" {
		t.Fatal("unconfigured domain inherited a hue")
	}
}

func TestCompileRejectsWithoutPartialValue(t *testing.T) {
	t.Parallel()
	for _, spec := range []presentation.Spec{
		{Domains: []presentation.DomainSpec{{ID: "ok"}, {ID: "-bad"}}},
		{Families: []presentation.Family{{ID: "other"}}},
		{Families: []presentation.Family{{ID: "ok", Label: "   "}}},
		{Types: []presentation.Override{{Type: "custom.arrival", Shape: "<svg>"}}},
		{Types: []presentation.Override{{Type: "custom.arrival", Shape: "star", LegacyKind: "bad"}}},
		{Types: []presentation.Override{{Type: "custom.arrival", Shape: "star", LegacyFamily: "family"}}},
	} {
		p, err := presentation.Compile(spec)
		if err == nil || len(p.Domains())+len(p.Families())+len(p.Overrides()) != 0 {
			t.Fatalf("partial acceptance %+v, %v", p, err)
		}
	}
}

func TestZeroPresentation(t *testing.T) {
	t.Parallel()
	var p presentation.Value
	if p.Hue("health") != "hx" || len(p.Domains())+len(p.Families())+len(p.Overrides()) != 0 {
		t.Fatal("zero value carries preferences")
	}
}
