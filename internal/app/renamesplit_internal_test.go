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

// sessionOf is an exercise session with the given payload id, a minimal payload
// activity.exercise-session took and each of its leaves takes. The binary keeps the
// type only in its history since card 195 split it, so the tests admit sessions
// under that (admitHeld).
func sessionOf(id, start string) record {
	return record{"activity.exercise-session", `{"id": "` + id + `", "activity_name": "rowing.machine", ` +
		`"effective_time_frame": {"time_interval": {"start_date_time": "` + start + `", "duration": {"value": 1200, "unit": "sec"}}}}`}
}

// sessions are four sessions, one per source's payload id prefix, as the live
// exercise sessions carry them.
var sessions = []record{
	sessionOf("garmin:session:1", "2026-03-01T07:00:00+11:00"),
	sessionOf("concept2:session:2", "2026-03-02T07:00:00+11:00"),
	sessionOf("technogym:3", "2026-03-03T07:00:00+11:00"),
	sessionOf("strong:session:4", "2026-03-04T07:00:00+11:00"),
}

var leaves = []facet.Type{
	"activity.exercise-session.garmin",
	"activity.exercise-session.concept2",
	"activity.exercise-session.technogym",
	"activity.exercise-session.strong",
}

// splitTable splits activity.exercise-session into the four leaves by payload id
// prefix, the technogym row also by source.
const splitTable = `{"renames": [
  {"from": "activity.exercise-session", "to": "activity.exercise-session.garmin", "select": {"pointer": "/id", "prefix": "garmin:session:"}},
  {"from": "activity.exercise-session", "to": "activity.exercise-session.concept2", "select": {"pointer": "/id", "prefix": "concept2:session:"}},
  {"from": "activity.exercise-session", "to": "activity.exercise-session.technogym", "select": {"source": "records", "pointer": "/id", "prefix": "technogym:"}},
  {"from": "activity.exercise-session", "to": "activity.exercise-session.strong", "select": {"pointer": "/id", "prefix": "strong:session:"}}
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
	before := f.admitHeld(t, heldDefs(t, "activity.exercise-session"), sessions)
	table := renameTableFile(t, f.dir, "split.json", splitTable)
	asOf, _, err := cli(t, f.cfgPath, "timeline", "--json", "--w", strconv.FormatInt(int64(before), 10))
	if err != nil {
		t.Fatalf("timeline before: %v", err)
	}
	if n := strings.Count(asOf, `"activity.exercise-session"`); n != 4 {
		t.Fatalf("timeline at W %d names activity.exercise-session %d times, want 4:\n%s", before, n, asOf)
	}

	out, _, err := cli(t, f.cfgPath, "rename", "--table", table)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	for _, leaf := range leaves {
		if row := reportRowTo(t, out, leaf); row[2] != "1" || row[3] != "0" {
			t.Errorf("dry run row %v, want 1 to carry and 0 refused", row)
		}
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Fatalf("the dry run wrote: W %d → %d", before, w)
	}

	if out, _, err = cli(t, f.cfgPath, "rename", "--table", table, "--yes"); err != nil {
		t.Fatalf("--yes: %v\n%s", err, out)
	}
	if !strings.Contains(out, "carried 4 facets") {
		t.Errorf("--yes said:\n%s", out)
	}

	l, err := ledger.Open(ctx, f.ledgerPath, "matt")
	if err != nil {
		t.Fatal(err)
	}
	for i, s := range sessions {
		var p struct{ ID string }
		if err := json.Unmarshal([]byte(s.payload), &p); err != nil {
			t.Fatal(err)
		}
		cur, err := l.CurrentVersion(ctx, facet.NewID(facet.SourceRef("records", p.ID)))
		if err != nil {
			t.Fatal(err)
		}
		want, err := facet.Canonicalize([]byte(s.payload))
		if err != nil {
			t.Fatal(err)
		}
		if cur.Type != leaves[i] || cur.ArtifactVersion != 2 || string(cur.Payload.Bytes()) != string(want.Bytes()) {
			t.Errorf("%s is %s v%d, want %s v2 with the payload unchanged", p.ID, cur.Type, cur.ArtifactVersion, leaves[i])
		}
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	// A read as-of before the run is unchanged.
	if again, _, err := cli(t, f.cfgPath, "timeline", "--json", "--w", strconv.FormatInt(int64(before), 10)); err != nil || again != asOf {
		t.Errorf("timeline at W %d changed across the run (%v):\nbefore %s\nafter  %s", before, err, asOf, again)
	}

	now, _, err := cli(t, f.cfgPath, "timeline", "--json")
	if err != nil {
		t.Fatal(err)
	}
	for _, leaf := range leaves {
		if !strings.Contains(now, `"`+string(leaf)+`"`) {
			t.Errorf("the timeline now does not show %s:\n%s", leaf, now)
		}
	}

	// A second dry run finds nothing left to carry.
	out, _, err = cli(t, f.cfgPath, "rename", "--table", table)
	if err != nil {
		t.Fatalf("second dry run: %v\n%s", err, out)
	}
	for _, leaf := range leaves {
		if row := reportRowTo(t, out, leaf); row[2] != "0" || row[3] != "0" {
			t.Errorf("second dry run row %v, want nothing to carry", row)
		}
	}
	if w := headOf(t, f.ledgerPath); w != before+4 {
		t.Errorf("W %d, want %d: one event per facet carried, none by the second dry run", w, before+4)
	}
}

// A facet the table does not place, by matching no row or more than one, stops the
// dry run and the real run, and nothing is written.
func TestRename_ASplitRefusesAFacetNoOneRowPlaces(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	before := f.admitHeld(t, heldDefs(t, "activity.exercise-session"), append(append([]record{}, sessions...), sessionOf("polar:session:5", "2026-03-05T07:00:00+11:00")))

	cases := []struct {
		name, table, says string
	}{
		{"unmatched", splitTable, "unmatched " + facet.SourceRef("records", "polar:session:5")},
		{"doubly matched", strings.Replace(splitTable, `"prefix": "strong:session:"`, `"prefix": "garmin:"`, 1),
			"doubly matched " + facet.SourceRef("records", "garmin:session:1")},
	}
	for _, c := range cases {
		table := renameTableFile(t, f.dir, strings.ReplaceAll(c.name, " ", "-")+".json", c.table)
		for _, args := range [][]string{{"rename", "--table", table}, {"rename", "--table", table, "--yes"}} {
			out, _, err := cli(t, f.cfgPath, args...)
			if err == nil || !strings.Contains(err.Error(), "match no rename or more than one") {
				t.Errorf("%s %v: %v, want a refusal", c.name, args[2:], err)
			}
			if !strings.Contains(out, c.says) {
				t.Errorf("%s %v: the report does not say %q:\n%s", c.name, args[2:], c.says, out)
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
		{"a split", splitTable, true},
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
	if err := json.Unmarshal([]byte(splitTable), &tbl); err != nil {
		t.Fatal(err)
	}
	session := func(source, payload string) facet.Facet {
		var f facet.Facet
		f.Type, f.Provenance.Source = "activity.exercise-session", source
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
		{"garmin", session("garmin", `{"id": "garmin:session:9"}`), []int{0}},
		{"technogym from records", session("records", `{"id": "technogym:9"}`), []int{2}},
		{"technogym from elsewhere", session("other", `{"id": "technogym:9"}`), nil},
		{"no id", session("records", `{"name": "x"}`), nil},
		{"an id that is not a string", session("records", `{"id": 7}`), nil},
		{"a prefix only part-way", session("records", `{"id": "strong:x"}`), nil},
		{"another type", func() facet.Facet {
			f := session("garmin", `{"id": "garmin:session:9"}`)
			f.Type = "activity.steps"
			return f
		}(), nil},
	}
	for _, c := range cases {
		if got := tbl.place(c.f); !slices.Equal(got, c.want) {
			t.Errorf("%s: placed on %v, want %v", c.name, got, c.want)
		}
	}
}
