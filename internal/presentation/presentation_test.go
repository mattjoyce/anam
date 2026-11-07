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
	// wantD, wantF and wantT came from the accessors under test, so check each
	// against a literal too: an accessor returning its backing slice would move
	// both sides of the comparison above.
	if d := p.Domains()[0]; d.ID != "custom" || d.Hue != 0 {
		t.Fatalf("Domains() leaked its backing slice: %+v", d)
	}
	if f := p.Families()[0]; f.Label != " <Label> " {
		t.Fatalf("Families() leaked its backing slice: %+v", f)
	}
	if o, ok := p.Lookup("custom.arrival"); !ok || o.Shape != "star" || p.Overrides()[0].Shape != "star" {
		t.Fatalf("Overrides() leaked its backing slice: %+v", o)
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

// Identifiers are unique within a section, not across sections: a domain and a
// family may share a name.
func TestCompileSameIDAcrossSections(t *testing.T) {
	t.Parallel()
	_, err := presentation.Compile(presentation.Spec{
		Domains:  []presentation.DomainSpec{{ID: "sleep"}},
		Families: []presentation.Family{{ID: "sleep"}},
	})
	if err != nil {
		t.Fatalf("a domain and a family named sleep were refused: %v", err)
	}
}
