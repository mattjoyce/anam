package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 255 (R4b): the engine catalog carries the rename cases slice 4 moves onto it.
// test.entry, only in its history, splits by a selector into the three course types,
// and test.sample, test.reading's earlier name, renames into a type that places some
// of its facets elsewhere. These run the real command over a ledger whose config
// reads fixture.Engine, so a slice 4 conversion has a working case to start from.

// newEngineLedgerFixture is newLedgerFixture over fixture.Engine.
func newEngineLedgerFixture(t *testing.T) ledgerFixture {
	t.Helper()
	dir := t.TempDir()
	f := ledgerFixture{dir: dir, cfgPath: filepath.Join(dir, "config.yaml"), ledgerPath: filepath.Join(dir, "ledger.db")}
	var c config.Config
	catalogtest.Engine.Config(t, &c)
	cfg := fmt.Sprintf("person_id: matt\nledger_path: %s\ndb_path: %s\ntimeout: %s\ndefinitions_dir: %s\nprojections_dir: %s\n",
		f.ledgerPath, filepath.Join(dir, "tracking.db"), testTimeout, c.DefinitionsDir, c.ProjectionsDir)
	if err := os.WriteFile(f.cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Create(context.Background(), f.ledgerPath, "matt")
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

// engineHeld is a registry over the newest history version of each type, from
// fixture.Engine, for admitting facets under a type no current manifest names.
func engineHeld(t *testing.T, types ...facet.Type) *def.Registry {
	t.Helper()
	newest := map[facet.Type]def.Stored{}
	for _, s := range catalogtest.Engine.Catalog(t).History() {
		if slices.Contains(types, s.Type) && s.Version > newest[s.Type].Version {
			newest[s.Type] = s
		}
	}
	var ds []def.Definition
	for _, typ := range types {
		s, ok := newest[typ]
		if !ok {
			t.Fatalf("no %s in the engine catalog's history", typ)
		}
		d, err := def.Parse(s.Body)
		if err != nil {
			t.Fatal(err)
		}
		ds = append(ds, d)
	}
	reg, err := def.NewRegistry(ds...)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

// entries are three course rows under test.entry, one for each course type; the
// stop carries no dose, which test.course.stopped refuses.
var entries = []record{
	{"test.entry", `{"id": "start:a/1", "date": "2026-03-01", "name": "a", "course": 1, "dose": "1 u"}`},
	{"test.entry", `{"id": "change:a/1", "date": "2026-03-05", "name": "a", "course": 1, "dose": "2 u"}`},
	{"test.entry", `{"id": "stop:a/1", "date": "2026-03-09", "name": "a", "course": 1, "note": "done"}`},
}

var entryLeaves = []facet.Type{"test.course.started", "test.course.changed", "test.course.stopped"}

const entrySplit = `{"renames": [
  {"from": "test.entry", "to": "test.course.started", "select": {"pointer": "/id", "prefix": "start:"}},
  {"from": "test.entry", "to": "test.course.changed", "select": {"pointer": "/id", "prefix": "change:"}},
  {"from": "test.entry", "to": "test.course.stopped", "select": {"source": "records", "pointer": "/id", "prefix": "stop:"}}
]}`

func TestRenameEngine_SplitsTheEntriesAcrossTheCourse(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newEngineLedgerFixture(t)
	before := f.admitHeld(t, engineHeld(t, "test.entry"), entries)
	table := renameTableFile(t, f.dir, "split.json", entrySplit)
	asOf, _, err := cli(t, f.cfgPath, "timeline", "--json", "--w", strconv.FormatInt(int64(before), 10))
	if err != nil {
		t.Fatalf("timeline before: %v", err)
	}
	if n := strings.Count(asOf, `"test.entry"`); n != 3 {
		t.Fatalf("timeline at W %d names test.entry %d times, want 3:\n%s", before, n, asOf)
	}

	out, _, err := cli(t, f.cfgPath, "rename", "--table", table)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	for _, leaf := range entryLeaves {
		if row := reportRowTo(t, out, leaf); row[2] != "1" || row[3] != "0" {
			t.Errorf("dry run row %v, want 1 to carry and 0 refused", row)
		}
	}
	if strings.Contains(out, " places facets by ") {
		t.Errorf("the course types place facets as test.entry did, but the report says otherwise:\n%s", out)
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Fatalf("the dry run wrote: W %d → %d", before, w)
	}

	if out, _, err = cli(t, f.cfgPath, "rename", "--table", table, "--yes"); err != nil || !strings.Contains(out, "carried 3 facets") {
		t.Fatalf("--yes: %v\n%s", err, out)
	}
	l, err := ledger.Open(ctx, f.ledgerPath, "matt")
	if err != nil {
		t.Fatal(err)
	}
	for i, e := range entries {
		var p struct{ ID string }
		if err := json.Unmarshal([]byte(e.payload), &p); err != nil {
			t.Fatal(err)
		}
		cur, err := l.CurrentVersion(ctx, facet.NewID(facet.SourceRef("records", p.ID)))
		if err != nil {
			t.Fatal(err)
		}
		want, err := facet.Canonicalize([]byte(e.payload))
		if err != nil {
			t.Fatal(err)
		}
		if cur.Type != entryLeaves[i] || cur.ArtifactVersion != 2 || string(cur.Payload.Bytes()) != string(want.Bytes()) {
			t.Errorf("%s is %s v%d, want %s v2 with the payload unchanged", p.ID, cur.Type, cur.ArtifactVersion, entryLeaves[i])
		}
		if old, err := l.Version(ctx, cur.ID, 1); err != nil || old.Type != "test.entry" {
			t.Errorf("%s v1 = %s, %v; want test.entry, kept as admitted", p.ID, old.Type, err)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	if again, _, err := cli(t, f.cfgPath, "timeline", "--json", "--w", strconv.FormatInt(int64(before), 10)); err != nil || again != asOf {
		t.Errorf("timeline at W %d changed across the run (%v):\nbefore %s\nafter  %s", before, err, asOf, again)
	}
	out, _, err = cli(t, f.cfgPath, "rename", "--table", table, "--yes")
	if err != nil {
		t.Fatalf("rerun: %v\n%s", err, out)
	}
	for _, leaf := range entryLeaves {
		if row := reportRowTo(t, out, leaf); row[2] != "0" || row[3] != "0" {
			t.Errorf("rerun row %v, want nothing to carry", row)
		}
	}
	if w := headOf(t, f.ledgerPath); w != before+3 {
		t.Errorf("W %d, want %d: one event per facet carried, none by the rerun", w, before+3)
	}
}

func TestRenameEngine_ASplitRefusesAnEntryNoOneRowPlaces(t *testing.T) {
	t.Parallel()
	f := newEngineLedgerFixture(t)
	odd := record{"test.entry", `{"id": "other:a/1", "date": "2026-03-10", "name": "a", "course": 1}`}
	before := f.admitHeld(t, engineHeld(t, "test.entry"), append(append([]record{}, entries...), odd))
	for _, c := range []struct{ name, table, says string }{
		{"unmatched", entrySplit, "unmatched " + facet.SourceRef("records", "other:a/1")},
		{"doubly matched", strings.Replace(entrySplit, `"prefix": "change:"`, `"prefix": "st"`, 1),
			"doubly matched " + facet.SourceRef("records", "start:a/1")},
	} {
		table := renameTableFile(t, f.dir, strings.ReplaceAll(c.name, " ", "-")+".json", c.table)
		for _, args := range [][]string{{"rename", "--table", table}, {"rename", "--table", table, "--yes"}} {
			out, _, err := cli(t, f.cfgPath, args...)
			if err == nil || !strings.Contains(err.Error(), "match no rename or more than one") || !strings.Contains(out, c.says) {
				t.Errorf("%s %v: %v, want a refusal saying %q:\n%s", c.name, args[2:], err, c.says, out)
			}
		}
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Errorf("a refused split wrote: W %d → %d", before, w)
	}
}

// samples are two test.sample facets said in London, one in summer at +01:00 and one
// at +00:00. test.sample read both wall clocks in UTC; test.reading reads them at the
// offset they name, so the first moves an hour and the second stays.
var samples = []record{
	{"test.sample", `{"id": "s:1", "at": "2026-10-19 18:00", "value": 5, "unit": "u",
		"source_time": {"value": "2026-10-19 18:00", "zone": "Europe/London", "offset": "+01:00", "basis": "source"}}`},
	{"test.sample", `{"id": "s:2", "at": "2026-11-19 18:00", "value": 6, "unit": "u",
		"source_time": {"value": "2026-11-19 18:00", "zone": "Europe/London", "offset": "+00:00", "basis": "source"}}`},
}

func TestRenameEngine_ARenameReplacesTheFacetsItMoves(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newEngineLedgerFixture(t)
	before := f.admitHeld(t, engineHeld(t, "test.sample"), samples)
	table := renameTableFile(t, f.dir, "sample.json", `{"renames": [{"from": "test.sample", "to": "test.reading"}]}`)
	asOf, _, err := cli(t, f.cfgPath, "timeline", "--json", "--w", strconv.FormatInt(int64(before), 10))
	if err != nil {
		t.Fatalf("timeline before: %v", err)
	}

	out, _, err := cli(t, f.cfgPath, "rename", "--table", table)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if want := "test.sample to test.reading places facets by /at (2006-01-02 15:04, Europe/Berlin), where test.sample " +
		"read /at (2006-01-02 15:04, UTC): 1 of 2 to carry move in fact time"; !strings.Contains(out, want) {
		t.Errorf("the dry run does not say %q:\n%s", want, out)
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Fatalf("the dry run wrote: W %d → %d", before, w)
	}
	if out, _, err = cli(t, f.cfgPath, "rename", "--table", table, "--yes"); err != nil || !strings.Contains(out, "carried 2 facets") {
		t.Fatalf("--yes: %v\n%s", err, out)
	}

	l, err := ledger.Open(ctx, f.ledgerPath, "matt")
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]time.Time{
		"s:1": time.Date(2026, 10, 19, 17, 0, 0, 0, time.UTC), // was 18:00Z, read in UTC
		"s:2": time.Date(2026, 11, 19, 18, 0, 0, 0, time.UTC), // unchanged
	} {
		cur, err := l.CurrentVersion(ctx, facet.NewID(facet.SourceRef("records", id)))
		if err != nil {
			t.Fatal(err)
		}
		if cur.Type != "test.reading" || cur.ArtifactVersion != 2 || !cur.FactTime.Equal(want) {
			t.Errorf("%s is %s v%d at %v, want test.reading v2 at %v", id, cur.Type, cur.ArtifactVersion, cur.FactTime, want)
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if again, _, err := cli(t, f.cfgPath, "timeline", "--json", "--w", strconv.FormatInt(int64(before), 10)); err != nil || again != asOf {
		t.Errorf("timeline at W %d changed across the run (%v):\nbefore %s\nafter  %s", before, err, asOf, again)
	}
	if w := headOf(t, f.ledgerPath); w != before+2 {
		t.Errorf("W %d, want %d", w, before+2)
	}
}
