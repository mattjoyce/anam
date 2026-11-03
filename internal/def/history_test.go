package def_test

import (
	"bytes"
	"encoding/json"
	"testing"
	"testing/fstest"

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

// A history body is words, not a contract: the store keeps each version in the format
// current when it was admitted, and a body from before card 52 does not parse today.
// ManifestsFS and HistoryFS read only its meta, so a catalog whose history holds one
// still loads, and the body comes back byte for byte. Engine's own history parses
// today, so the case runs over a copy of it with one such body added.
func TestManifestsFS_KeepsAHistoryBodyTheContractRefuses(t *testing.T) {
	t.Parallel()
	const name = "history/test.lenient.v1.json"
	body := []byte(`{"meta": {"facet_type": "test.lenient", "definition_version": 1, "classification": "Normal",
  "validation_mode": "lenient", "source_schema": "test/lenient",
  "time_semantics": {"fact_time": {"pointer": "/at", "layout": "2006-01-02 15:04", "zone": "UTC"}},
  "identity_key": ["/at"]},
  "schema": {"required": ["/at"], "properties": {"/at": {}}}}`)
	if _, err := def.Parse(body); err == nil {
		t.Fatal("the lenient body parses as a contract; the case proves nothing")
	}
	root := copyOf(t, fixtureDefs())
	root[name] = &fstest.MapFile{Data: body}

	all, err := def.ManifestsFS(root)
	if err != nil {
		t.Fatalf("ManifestsFS refused a catalog whose history holds a body the contract refuses: %v", err)
	}
	history, err := def.HistoryFS(root)
	if err != nil {
		t.Fatalf("HistoryFS: %v", err)
	}
	for label, set := range map[string][]def.Stored{"ManifestsFS": all, "HistoryFS": history} {
		found := false
		for _, s := range set {
			if s.Type == "test.lenient" && s.Version == 1 {
				found = true
				if !bytes.Equal(s.Body, body) {
					t.Errorf("%s keeps test.lenient v1 as %s, want the file's bytes", label, s.Body)
				}
			}
		}
		if !found {
			t.Errorf("%s does not carry test.lenient v1", label)
		}
	}
	if len(all) != 17 || len(history) != 7 {
		t.Errorf("ManifestsFS %d, HistoryFS %d; want 17 and 7, the fixture's and the one added", len(all), len(history))
	}
	reg, err := def.BundledFS(root)
	if err != nil {
		t.Fatalf("BundledFS: %v", err)
	}
	if _, ok := reg.Lookup("test.lenient"); ok {
		t.Error("BundledFS carries test.lenient, which only history names")
	}
}
