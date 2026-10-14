package def_test

import (
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// Card 111: every version a facet may have been admitted under comes back with the
// binary, so a rebuilt operational store holds the history and an old facet still
// reads against its own words. Bundled stays the current contracts alone.
func TestManifests_CarryTheHistory(t *testing.T) {
	t.Parallel()
	all, err := def.Manifests()
	if err != nil {
		t.Fatalf("Manifests: %v", err)
	}
	held := map[facet.Type]map[int]bool{}
	for _, s := range all {
		if held[s.Type] == nil {
			held[s.Type] = map[int]bool{}
		}
		if held[s.Type][s.Version] {
			t.Errorf("%s v%d is carried twice", s.Type, s.Version)
		}
		held[s.Type][s.Version] = true
	}
	for typ, versions := range map[facet.Type][]int{
		"health.blood-pressure": {1, 2, 3, 4, 5},
		"health.measurement":    {1, 2},
		"health.weight":         {1, 2, 3},
		"exercise.session":      {1, 2, 3, 4, 5},
		"note.annotation":       {1, 2, 3},
	} {
		for _, v := range versions {
			if !held[typ][v] {
				t.Errorf("%s v%d is not carried", typ, v)
			}
		}
	}

	reg, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	if bp, ok := reg.Lookup("health.blood-pressure"); !ok || bp.Version != 5 {
		t.Errorf("Bundled bp = v%d, %v; want the current v5 alone", bp.Version, ok)
	}
	if _, ok := reg.Lookup("health.measurement"); ok {
		t.Error("Bundled carries health.measurement, which left the bundle; its history must not bring it back")
	}
	history, err := def.History()
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range history {
		if cur, ok := reg.Lookup(h.Type); ok && h.Version >= cur.Version {
			t.Errorf("history %s v%d is not older than the current v%d", h.Type, h.Version, cur.Version)
		}
	}
}
