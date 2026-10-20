package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 140: a rename run carries every live facet of a type into another type as its
// next version, through the gate, keeping the id. The dry run reports and writes
// nothing, a refusal anywhere stops the run before anything is written, a rerun is
// all no-op, and the first version stays, so anam-time is kept.

// illnesses are three self-log illnesses as records. health.illness-started and
// health.illness-ended share keying, series and schema, which is the shape of a rename:
// the type moves and nothing else does.
var illnesses = []record{
	{"health.illness-started", `{"id": "self-log:ill/flu/1", "date": "2026-03-04", "episode": "flu"}`},
	{"health.illness-started", `{"id": "self-log:ill/cold/1", "date": "2026-05-10", "episode": "cold", "note": "mild"}`},
	{"health.illness-started", `{"id": "self-log:ill/flu/2", "date": "2026-08-01", "episode": "flu"}`},
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
	before := f.admit(t, illnesses)
	table := renameTableFile(t, f.dir, "renames.json", `{"renames": [{"from": "health.illness-started", "to": "health.illness-ended"}]}`)

	out, _, err := cli(t, f.cfgPath, "rename", "--table", table)
	if err != nil {
		t.Fatalf("dry run: %v\n%s", err, out)
	}
	if row := reportRow(t, out, "health.illness-started"); row[1] != "health.illness-ended" || row[2] != "3" || row[3] != "0" {
		t.Errorf("dry run row %v, want health.illness-ended 3 0", row)
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
	id := facet.NewID(facet.SourceRef("records", "self-log:ill/flu/1"))
	cur, err := l.CurrentVersion(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Type != "health.illness-ended" || cur.ArtifactVersion != 2 || cur.Provenance.Source != "records" {
		t.Errorf("current is %s v%d from %s, want health.illness-ended v2 from records, under the same id", cur.Type, cur.ArtifactVersion, cur.Provenance.Source)
	}
	old, err := l.Version(ctx, id, 1)
	if err != nil || old.Type != "health.illness-started" {
		t.Errorf("v1 = %s, %v; want health.illness-started, kept as admitted", old.Type, err)
	}

	// The run again finds nothing left under the old type, and writes nothing.
	out, _, err = cli(t, f.cfgPath, "rename", "--table", table, "--yes")
	if err != nil {
		t.Fatalf("rerun: %v\n%s", err, out)
	}
	if row := reportRow(t, out, "health.illness-started"); row[2] != "0" || row[3] != "0" {
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
	trip := record{"place.departed", `{"id": "self-log:trip/1/departed", "date": "2026-04-01", "trip": "tokyo", "destination": "Tokyo"}`}
	before := f.admit(t, append(append([]record{}, illnesses...), trip))

	// health.weight is keyed by id and dated by date, as an illness is, but its layout
	// carries the time of day: a different fact-time rule, so different keying.
	mis := renameTableFile(t, f.dir, "mis.json", `{"renames": [{"from": "health.illness-started", "to": "health.weight"}]}`)
	if _, _, err := cli(t, f.cfgPath, "rename", "--table", mis, "--yes"); err == nil || !strings.Contains(err.Error(), "ids would not meet") {
		t.Errorf("different keying: %v, want a refusal saying the ids would not meet", err)
	}

	// place.arrived keys as place.departed does but has no destination field, so the
	// departure's payload fails its schema: reported per facet, nothing written.
	bad := renameTableFile(t, f.dir, "bad.json", `{"renames": [{"from": "place.departed", "to": "place.arrived"}]}`)
	out, _, err := cli(t, f.cfgPath, "rename", "--table", bad, "--yes")
	if err == nil || !strings.Contains(err.Error(), "would refuse") {
		t.Errorf("a payload the new schema refuses: %v\n%s", err, out)
	}
	if !strings.Contains(out, "refused "+facet.SourceRef("records", "self-log:trip/1/departed")) {
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
