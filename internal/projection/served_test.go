package projection_test

import (
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/facet"
)

// A type with no projection cannot be reached by the query route, which runs through
// one, so an agent has only a whole-history read. Over the engine fixture every
// Definition's type is served by a current template, the course series through a
// prefix, and a template names only types the catalog defines. Which types a
// deployment leaves unserved, and why, is the deployment's to test.
func TestEveryFixtureTypeIsServed(t *testing.T) {
	t.Parallel()
	defs := catalogtest.Engine.Definitions(t)
	projs := catalogtest.Engine.Projections(t)
	all := defs.All()
	if len(all) != 10 {
		t.Fatalf("the engine fixture holds %d types, want 10", len(all))
	}
	served := map[facet.Type][]string{}
	for _, tpl := range projs.All() {
		for _, typ := range tpl.Covers(all) {
			served[typ] = append(served[typ], tpl.Name)
		}
		for _, typ := range tpl.Types {
			if !defs.Has(typ) {
				t.Errorf("%s v%d names %s, which has no Definition", tpl.Name, tpl.Version, typ)
			}
		}
	}
	for _, d := range all {
		if len(served[d.Type]) == 0 {
			t.Errorf("%s is served by no current template", d.Type)
		}
	}
	// What each template covers: everything and normal reach every type, the course
	// series through their prefix, and the narrow two only their own.
	for name, want := range map[string][]facet.Type{
		"measures": {"test.measure"},
		"course":   {"test.course.changed", "test.course.started", "test.course.stopped"},
	} {
		tpl, err := projs.Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := tpl.Covers(all); !reflect.DeepEqual(got, want) {
			t.Errorf("%s covers %v, want %v", name, got, want)
		}
	}
	for _, name := range []string{"everything", "normal"} {
		tpl, err := projs.Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(tpl.Covers(all)); got != len(all) {
			t.Errorf("%s covers %d of the %d types", name, got, len(all))
		}
		if !reflect.DeepEqual(tpl.Prefixes, []string{"test.course."}) {
			t.Errorf("%s names the prefixes %v, want the course series' test.course.", name, tpl.Prefixes)
		}
	}
}
