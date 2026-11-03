package def_test

import (
	"encoding/json"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// Card 111: every version a facet may have been admitted under comes back with the
// catalog, so a rebuilt operational store holds the history and an old facet still
// reads against its own words. BundledFS stays the current contracts alone. Over the
// engine fixture: a v1 under history/ loads, is named by its file, and is not parsed
// as a contract, so a type only history names (a retired or renamed one) does not come
// back as current.
func TestManifestsFS_CarryTheHistory(t *testing.T) {
	t.Parallel()
	all, err := def.ManifestsFS(fixtureDefs())
	if err != nil {
		t.Fatalf("ManifestsFS: %v", err)
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
		// The type and version are the file's: history/<type>.v<n>.json, or <type>.json
		// for the current body, whose meta says the same.
		var m struct {
			Meta struct {
				Type    facet.Type `json:"facet_type"`
				Version int        `json:"definition_version"`
			} `json:"meta"`
		}
		if err := json.Unmarshal(s.Body, &m); err != nil {
			t.Fatalf("%s v%d: %v", s.Type, s.Version, err)
		}
		if m.Meta.Type != s.Type || m.Meta.Version != s.Version {
			t.Errorf("stored as %s v%d, body says %s v%d", s.Type, s.Version, m.Meta.Type, m.Meta.Version)
		}
	}
	for typ, versions := range map[facet.Type][]int{
		"test.measure":   {1, 2},
		"test.reading":   {1, 2},
		"test.templated": {1, 2},
		// Only history names these: a split source, a rename's old name and a
		// retired stop.
		"test.entry":   {1},
		"test.sample":  {1},
		"test.retired": {1},
	} {
		for _, v := range versions {
			if !held[typ][v] {
				t.Errorf("%s v%d is not carried", typ, v)
			}
		}
	}
	if got, want := len(all), 16; got != want {
		t.Errorf("ManifestsFS carries %d bodies, want the fixture's %d (10 current, 6 history)", got, want)
	}

	reg, err := def.BundledFS(fixtureDefs())
	if err != nil {
		t.Fatal(err)
	}
	if m, ok := reg.Lookup("test.measure"); !ok || m.Version != 2 {
		t.Errorf("BundledFS test.measure = v%d, %v; want the current v2 alone", m.Version, ok)
	}
	for _, old := range []facet.Type{"test.entry", "test.sample", "test.retired"} {
		if _, ok := reg.Lookup(old); ok {
			t.Errorf("BundledFS carries %s, which only history names; its history must not bring it back", old)
		}
	}
	history, err := def.HistoryFS(fixtureDefs())
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 6 {
		t.Errorf("HistoryFS = %d bodies, want the fixture's 6", len(history))
	}
	for _, h := range history {
		if cur, ok := reg.Lookup(h.Type); ok && h.Version >= cur.Version {
			t.Errorf("history %s v%d is not older than the current v%d", h.Type, h.Version, cur.Version)
		}
	}
}
