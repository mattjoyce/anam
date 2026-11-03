package app

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 140: a rename run carries every live facet of a type into another type as its
// next version, through the gate, keeping the id. The dry run reports and writes
// nothing, a refusal anywhere stops the run before anything is written, a rerun is
// all no-op, and the first version stays, so anam-time is kept.

// stops are three course stops as records, under test.retired, the name the stop had
// before it became test.course.stopped. The two share keying, series and schema, which
// is the shape of a rename: the type moves and nothing else does.
var stops = []record{
	{"test.retired", `{"id": "stop:flu/1", "date": "2026-03-04", "name": "flu", "course": 1}`},
	{"test.retired", `{"id": "stop:cold/1", "date": "2026-05-10", "name": "cold", "course": 1, "note": "mild"}`},
	{"test.retired", `{"id": "stop:flu/2", "date": "2026-08-01", "name": "flu", "course": 2}`},
}

func renameTableFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	writeDoc(t, p, body)
	return p
}

// reportRow is the report's row for a type left, split into fields.
func reportRow(t *testing.T, out string, from facet.Type) []string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, string(from)+" ") {
			return strings.Fields(line)
		}
	}
	t.Fatalf("no report row for %s in:\n%s", from, out)
	return nil
}

func TestRename_CarriesFacetsAndKeepsTheFirstVersion(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	before := f.admitHeld(t, heldDefs(t, "test.retired"), stops)
	table := renameTableFile(t, f.dir, "renames.json", `{"renames": [{"from": "test.retired", "to": "test.course.stopped"}]}`)

	out, _, err := cli(t, f.cfgPath, "rename", "--table", table)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if row := reportRow(t, out, "test.retired"); row[1] != "test.course.stopped" || row[2] != "3" || row[3] != "0" {
		t.Errorf("dry run row %v, want test.course.stopped 3 0", row)
	}
	if !strings.Contains(out, "dry run: nothing written") {
		t.Errorf("the dry run did not say so:\n%s", out)
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Fatalf("the dry run wrote: W %d → %d", before, w)
	}

	out, _, err = cli(t, f.cfgPath, "rename", "--table", table, "--yes")
	if err != nil {
		t.Fatalf("--yes: %v\n%s", err, out)
	}
	if !strings.Contains(out, "carried 3 facets") {
		t.Errorf("--yes said:\n%s", out)
	}
	if w := headOf(t, f.ledgerPath); w != before+3 {
		t.Errorf("W %d, want %d: one event per facet carried", w, before+3)
	}

	ctx := context.Background()
	l, err := ledger.Open(ctx, f.ledgerPath, "matt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	id := facet.NewID(facet.SourceRef("records", "stop:flu/1"))
	cur, err := l.CurrentVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Type != "test.course.stopped" || cur.ArtifactVersion != 2 || cur.Provenance.Source != "records" {
		t.Errorf("current is %s v%d from %s, want test.course.stopped v2 from records, under the same id", cur.Type, cur.ArtifactVersion, cur.Provenance.Source)
	}
	old, err := l.Version(ctx, id, 1)
	if err != nil || old.Type != "test.retired" {
		t.Errorf("v1 = %s, %v; want test.retired, kept as admitted", old.Type, err)
	}

	// The run again finds nothing left under the old type, and writes nothing.
	out, _, err = cli(t, f.cfgPath, "rename", "--table", table, "--yes")
	if err != nil {
		t.Fatalf("rerun: %v\n%s", err, out)
	}
	if row := reportRow(t, out, "test.retired"); row[2] != "0" || row[3] != "0" {
		t.Errorf("rerun row %v, want nothing to carry", row)
	}
	if w := headOf(t, f.ledgerPath); w != before+3 {
		t.Errorf("the rerun wrote: W %d", w)
	}
}

// A run is refused before anything is written: by the keying check when the two
// types would not give the same ids, by the gate when a payload does not fit the new
// Definition, and by the table's loader when the table is not one step.
func TestRename_RefusesBeforeWriting(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	start := record{"test.course.started", `{"id": "start:trip/1", "date": "2026-04-01", "name": "trip", "course": 1, "dose": "1 u"}`}
	f.admitHeld(t, heldDefs(t, "test.retired"), stops)
	before := f.admit(t, []record{start})

	// test.measure composes its id from /at and /seq, where a stop's is its id: another
	// identity key, so the ids would not meet. A different fact-time rule alone is no
	// refusal (card 195; TestRename_ARenameReplacesTheFacetsItMoves).
	mis := renameTableFile(t, f.dir, "mis.json", `{"renames": [{"from": "test.retired", "to": "test.measure"}]}`)
	if _, _, err := cli(t, f.cfgPath, "rename", "--table", mis, "--yes"); err == nil || !strings.Contains(err.Error(), "ids would not meet") {
		t.Errorf("different keying: %v, want a refusal saying the ids would not meet", err)
	}

	// test.course.stopped keys as test.course.started does but has no dose field, so the
	// start's payload fails its schema: reported per facet, nothing written.
	bad := renameTableFile(t, f.dir, "bad.json", `{"renames": [{"from": "test.course.started", "to": "test.course.stopped"}]}`)
	out, _, err := cli(t, f.cfgPath, "rename", "--table", bad, "--yes")
	if err == nil || !strings.Contains(err.Error(), "would refuse") {
		t.Errorf("a payload the new schema refuses: %v\n%s", err, out)
	}
	if !strings.Contains(out, "refused "+facet.SourceRef("records", "start:trip/1")) {
		t.Errorf("the refusal does not name the facet:\n%s", out)
	}
	if w := headOf(t, f.ledgerPath); w != before {
		t.Errorf("a refused run wrote: W %d → %d", before, w)
	}

	tables := map[string]string{
		"a-chain":      `{"renames": [{"from": "a.b", "to": "c.d"}, {"from": "c.d", "to": "e.f"}]}`,
		"left-twice":   `{"renames": [{"from": "a.b", "to": "c.d"}, {"from": "a.b", "to": "e.f"}]}`,
		"unknown-key":  `{"renames": [{"from": "a.b", "to": "c.d", "reshape": "x"}]}`,
		"goes-nowhere": `{"renames": [{"from": "a.b", "to": "a.b"}]}`,
		"empty":        `{"renames": []}`,
	}
	for name, body := range tables {
		p := renameTableFile(t, f.dir, name+".json", body)
		if _, _, err := cli(t, f.cfgPath, "rename", "--table", p); err == nil {
			t.Errorf("%s: the table was accepted", name)
		}
	}
}

// Card 195: a rename may move a facet in fact time, when the new type reads its clock
// another way. The dry run says how many move, and an earlier watermark's read is kept.

// samples are two test.sample facets said in London, one in summer at +01:00 and one
// at +00:00. test.sample read both wall clocks in UTC; test.reading reads them at the
// offset they name, so the first moves an hour and the second stays.
var samples = []record{
	{"test.sample", `{"id": "s:1", "at": "2026-10-19 18:00", "value": 5, "unit": "u",
		"source_time": {"value": "2026-10-19 18:00", "zone": "Europe/London", "offset": "+01:00", "basis": "source"}}`},
	{"test.sample", `{"id": "s:2", "at": "2026-11-19 18:00", "value": 6, "unit": "u",
		"source_time": {"value": "2026-11-19 18:00", "zone": "Europe/London", "offset": "+00:00", "basis": "source"}}`},
}

func TestRename_ARenameReplacesTheFacetsItMoves(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	f := newLedgerFixture(t)
	before := f.admitHeld(t, heldDefs(t, "test.sample"), samples)
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

// heldDefs is a registry holding each named type as the catalog's history keeps it, the
// newest version of it, as a ledger written before a rename holds the type.
func heldDefs(t *testing.T, types ...facet.Type) *def.Registry {
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
			t.Fatalf("no %s in the history", typ)
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
