package app

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 194: a rename table splits one type into several by a selector. Every live
// facet of the type split must match exactly one row; one that matches none, or
// more than one, stops the dry run and the real run before anything is written.

// entries are three course rows under test.entry, which the engine catalog keeps only
// in its history, so the tests admit them under that (admitHeld). The stop carries no
// dose, which test.course.started and .changed require and test.course.stopped refuses.
var entries = []record{
	{"test.entry", `{"id": "start:a/1", "date": "2026-03-01", "name": "a", "course": 1, "dose": "1 u"}`},
	{"test.entry", `{"id": "change:a/1", "date": "2026-03-05", "name": "a", "course": 1, "dose": "2 u"}`},
	{"test.entry", `{"id": "stop:a/1", "date": "2026-03-09", "name": "a", "course": 1, "note": "done"}`},
}

var entryLeaves = []facet.Type{"test.course.started", "test.course.changed", "test.course.stopped"}

// entrySplit splits test.entry into the three course types by payload id prefix, the
// stop's row also by source.
const entrySplit = `{"renames": [
  {"from": "test.entry", "to": "test.course.started", "select": {"pointer": "/id", "prefix": "start:"}},
  {"from": "test.entry", "to": "test.course.changed", "select": {"pointer": "/id", "prefix": "change:"}},
  {"from": "test.entry", "to": "test.course.stopped", "select": {"source": "records", "pointer": "/id", "prefix": "stop:"}}
]}`

// reportRowTo is the report's row for a rename arriving at to, split into fields.
func reportRowTo(t *testing.T, out string, to facet.Type) []string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if fs := strings.Fields(line); len(fs) == 5 && fs[1] == string(to) {
			return fs
		}
	}
	t.Fatalf("no report row arriving at %s in:\n%s", to, out)
	return nil
}

func TestRename_SplitsATypeByASelector(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newLedgerFixture(t)
	before := f.admitHeld(t, heldDefs(t, "test.entry"), entries)
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
	now, _, err := cli(t, f.cfgPath, "timeline", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, leaf := range entryLeaves {
		if !strings.Contains(now, `"`+string(leaf)+`"`) {
			t.Errorf("the timeline now does not show %s:\n%s", leaf, now)
		}
	}

	// A rerun finds nothing left to carry.
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

// A facet the table does not place, by matching no row or more than one, stops the
// dry run and the real run, and nothing is written.
func TestRename_ASplitRefusesAFacetNoOneRowPlaces(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	odd := record{"test.entry", `{"id": "other:a/1", "date": "2026-03-10", "name": "a", "course": 1}`}
	before := f.admitHeld(t, heldDefs(t, "test.entry"), append(append([]record{}, entries...), odd))
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

func TestRenameTable_SelectRules(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cases := []struct {
		name, body string
		ok         bool
	}{
		{"a split", entrySplit, true},
		{"source alone", `{"renames": [{"from": "a.b", "to": "a.b.c", "select": {"source": "records"}}]}`, true},
		{"empty select", `{"renames": [{"from": "a.b", "to": "a.b.c", "select": {}}]}`, false},
		{"pointer without prefix", `{"renames": [{"from": "a.b", "to": "a.b.c", "select": {"pointer": "/id"}}]}`, false},
		{"prefix without pointer", `{"renames": [{"from": "a.b", "to": "a.b.c", "select": {"prefix": "x:"}}]}`, false},
		{"not a pointer", `{"renames": [{"from": "a.b", "to": "a.b.c", "select": {"pointer": "id", "prefix": "x:"}}]}`, false},
		{"unknown select key", `{"renames": [{"from": "a.b", "to": "a.b.c", "select": {"source": "records", "suffix": "x"}}]}`, false},
		{"whole and split", `{"renames": [{"from": "a.b", "to": "c.d"}, {"from": "a.b", "to": "e.f", "select": {"source": "records"}}]}`, false},
		{"split and whole", `{"renames": [{"from": "a.b", "to": "e.f", "select": {"source": "records"}}, {"from": "a.b", "to": "c.d"}]}`, false},
		{"a row repeated", `{"renames": [{"from": "a.b", "to": "e.f", "select": {"source": "records"}}, {"from": "a.b", "to": "e.f", "select": {"source": "records"}}]}`, false},
		{"a split chain", `{"renames": [{"from": "a.b", "to": "c.d", "select": {"source": "records"}}, {"from": "c.d", "to": "e.f"}]}`, false},
	}
	for _, c := range cases {
		p := renameTableFile(t, dir, strings.ReplaceAll(c.name, " ", "-")+".json", c.body)
		if _, err := loadRenameTable(p); (err == nil) != c.ok {
			t.Errorf("%s: err %v, want ok %v", c.name, err, c.ok)
		}
	}
}

func TestRenameTable_PlacesAFacetByWhatItSays(t *testing.T) {
	t.Parallel()
	var tbl renameTable
	if err := json.Unmarshal([]byte(entrySplit), &tbl); err != nil {
		t.Fatal(err)
	}
	entry := func(source, payload string) facet.Facet {
		var f facet.Facet
		f.Type, f.Provenance.Source = "test.entry", source
		p, err := facet.Canonicalize([]byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		f.Payload = p
		return f
	}
	cases := []struct {
		name string
		f    facet.Facet
		want []int
	}{
		{"a start from anywhere", entry("other", `{"id": "start:a/9"}`), []int{0}},
		{"a stop from records", entry("records", `{"id": "stop:a/9"}`), []int{2}},
		{"a stop from elsewhere", entry("other", `{"id": "stop:a/9"}`), nil},
		{"no id", entry("records", `{"name": "x"}`), nil},
		{"an id that is not a string", entry("records", `{"id": 7}`), nil},
		{"a prefix only part-way", entry("records", `{"id": "change"}`), nil},
		{"another type", func() facet.Facet {
			f := entry("other", `{"id": "start:a/9"}`)
			f.Type = "test.course.started"
			return f
		}(), nil},
	}
	for _, c := range cases {
		if got := tbl.place(c.f); !slices.Equal(got, c.want) {
			t.Errorf("%s: placed on %v, want %v", c.name, got, c.want)
		}
	}
}
