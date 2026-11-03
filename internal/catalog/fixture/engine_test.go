package fixture_test

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"path"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/fixture"
)

// engineFiles is every file fixture.Engine holds, and the engine tests each one is
// there for (card 255, R4b slice 4). A file added or removed changes this list and
// the definition set version catalogtest pins, so either is a decision, not drift.
var engineFiles = map[string][]string{
	"definitions": {
		"history/test.entry.v1.json",     // app renamesplit: a generic source a selector split carries to the three course types
		"history/test.measure.v1.json",   // def versions and history: an earlier version that loads, named by its file
		"history/test.reading.v1.json",   // def guard: v2 widens the fact time with a zone pointer
		"history/test.retired.v1.json",   // def retired, serve retired_type: a retired type that closes its series (card 187)
		"history/test.sample.v1.json",    // app rename (card 195): test.reading's old name, read in UTC; the rename re-places some facets
		"history/test.templated.v1.json", // def factzone, idtemplate: v2 appends an optional offset; an old id holds
		"record.document.json",           // daemon documents, inspect document: the structural type, in UTC
		"record.edge.json",               // catchup, query edges, inspect links, migrate: the structural type, in UTC
		"test.course.changed.json",       // projection snapshot: a middle statement of the series
		"test.course.started.json",       // projection snapshot, app rename: opens the series
		"test.course.stopped.json",       // registry Closes, snapshot: closes the series; a rename target for test.retired
		"test.labelled.json",             // def and inspect label: fields that may be absent
		"test.measure.json",              // gate, read, serve, query: a two-field identity key, source_time v1, Normal
		"test.pair.json",                 // bench: two numbers and a time, keyed by the time, nothing else required
		"test.reading.json",              // def factzone, sourcetime, inspect zone: a wall clock read at its offset
		"test.templated.json",            // def idtemplate and identityfields, gate identity_template, inspect entry form
	},
	"projections": {
		"course.json",            // serve snapshot: the latest statement per course, stops gone
		"everything.json",        // projection served-types, inspect timeline: every type, the series by prefix
		"history/course.v1.json", // serve retired_type, projection register: an earlier version naming test.retired
		"measures.json",          // serve and receipts: a log of one type
		"normal.json",            // projection max_classification: a Normal ceiling over every type
	},
}

// TestEngine_Layout pins the engine catalog's files, current and history.
func TestEngine_Layout(t *testing.T) {
	t.Parallel()
	defs, projs := fixture.Engine()
	for name, root := range map[string]fs.FS{"definitions": defs, "projections": projs} {
		var got []string
		err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				got = append(got, p)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		want := slices.Clone(engineFiles[name])
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("%s = %v\nwant %v", name, got, want)
		}
	}
}

// TestEngine_RecordTypesAreMinimals checks the engine's record.edge and
// record.document are Minimal's bytes, so the structural types read in UTC in both.
func TestEngine_RecordTypesAreMinimals(t *testing.T) {
	t.Parallel()
	engine, _ := fixture.Engine()
	minimal, _ := fixture.Minimal()
	for _, name := range []string{"record.document.json", "record.edge.json"} {
		got, err := fs.ReadFile(engine, name)
		if err != nil {
			t.Fatal(err)
		}
		want, err := fs.ReadFile(minimal, name)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("%s differs from Minimal's", name)
		}
	}
}

// personal is what a personal catalog names and the engine catalog must not: the
// deployment's domain heads other than record, and the zone the personal catalog
// reads in.
var personal = []string{"health.", "activity.", "place.", "mind.", "org.", "money.", "social.", "media.",
	"work.", "Australia/Sydney"}

// TestEngine_NamesNothingPersonal fails on a file of the engine catalog that names a
// personal type or zone, in its name, a facet_type, a projection's types or anywhere
// in its bytes, and on a type outside test. and record. .
func TestEngine_NamesNothingPersonal(t *testing.T) {
	t.Parallel()
	defs, projs := fixture.Engine()
	for _, root := range []fs.FS{defs, projs} {
		err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			body, err := fs.ReadFile(root, p)
			if err != nil {
				return err
			}
			for _, word := range personal {
				if strings.Contains(p, word) || bytes.Contains(body, []byte(word)) {
					t.Errorf("%s names %q", p, word)
				}
			}
			var m struct {
				Meta struct {
					Type string `json:"facet_type"`
				} `json:"meta"`
				Types []string `json:"types"`
			}
			if err := json.Unmarshal(body, &m); err != nil {
				t.Errorf("%s: %v", p, err)
				return nil
			}
			named := m.Types
			if m.Meta.Type != "" {
				named = append(named, m.Meta.Type)
				if want := m.Meta.Type; !strings.HasPrefix(path.Base(p), want+".") {
					t.Errorf("%s declares %s", p, want)
				}
			}
			for _, typ := range named {
				if !strings.HasPrefix(typ, "test.") && !strings.HasPrefix(typ, "record.") {
					t.Errorf("%s names %s, outside test. and record.", p, typ)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}
