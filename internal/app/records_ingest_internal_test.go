package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"

	"github.com/mattjoyce/anam/internal/capture/records"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The records source is tested through the real gate here rather than beside the
// worker, because a capture package may not import the store (scripts/check.sh), and
// because the thing under test is the wiring: the bundled definitions, the token
// scopes in mvpScopes, and the worker, together.

const labFile = `document:
  name: 2026-01-30_pathology_lipid_studies.pdf
  sha256: 0000000000000000000000000000000000000000000000000000000000000000
extracted:
  method: scripts/extract-acl-pathology.py
  version: 1
records:
  - type: lab.result
    id: acl:39986323/total-chol
    collected: "2026-01-30 08:53"
    lab_number: "39986323"
    analyte: Total Chol.
    value: "5.0"
    unit: mmol/L
    range: "< 5.6"
  - type: lab.result
    id: acl:22200789/total-chol
    collected: "2024-09-10 07:46"
    lab_number: "22200789"
    analyte: Total Chol.
    value: "4.7"
    unit: mmol/L
    range: "< 5.6"
`

func write(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return p
}

// Through the real gate with the bundled definitions: the two records are admitted at
// their collection times, a re-scan is a no-op, and a changed value supersedes.
func TestRecords_AdmitThroughTheGate(t *testing.T) {
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	scopes := mvpScopes
	g := gate.New(l, defs, scopes, clock.Real{}, "matt")

	dir := t.TempDir()
	write(t, dir, "lipids.yaml", labFile)
	admit := func() map[gate.Outcome]int {
		envs, err := records.Scan(dir, "matt", defs)
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		tally := map[gate.Outcome]int{}
		for _, env := range envs {
			b, err := g.Resolve(env)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			res, _, err := g.Commit(ctx, env, b)
			if err != nil {
				t.Fatalf("Commit %s: %v", env.SourceRef, err)
			}
			tally[res.Outcome]++
		}
		return tally
	}

	if got := admit(); got[gate.OutcomeAdmitted] != 2 {
		t.Fatalf("first pass: %v, want 2 admitted", got)
	}
	if got := admit(); got[gate.OutcomeNoOp] != 2 {
		t.Fatalf("second pass: %v, want 2 no-op", got)
	}

	res, err := fold.Read(ctx, l)
	if err != nil {
		t.Fatalf("fold.Read: %v", err)
	}
	// lab.result reads its clock in Australia/Sydney (AEST, +10 in September), so the
	// 07:46 collection lands at 21:46 UTC the evening before.
	if res.Value.Len() != 2 || res.Live[0].FactTime.UTC().Format("2006-01-02 15:04") != "2024-09-09 21:46" {
		t.Fatalf("canonical: %d facets, first at %s", res.Value.Len(), res.Live[0].FactTime)
	}

	// A corrected value under the same id is a new version, not a new fact.
	write(t, dir, "lipids.yaml", labFile[:len(labFile)-len("    range: \"< 5.6\"\n")]+"    range: \"< 5.5\"\n")
	if got := admit(); got[gate.OutcomeSuperseded] != 1 || got[gate.OutcomeNoOp] != 1 {
		t.Fatalf("third pass: %v, want 1 superseded and 1 no-op", got)
	}
}

// A blood-pressure record's identity is what its Definition composes, date_excel then
// reading, whatever its id says: the worker asks the Definition, as the gate does, so
// the two never disagree and the record's id is just another field.
func TestRecords_BloodPressureIdentityComposes(t *testing.T) {
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	g := gate.New(l, defs, mvpScopes, clock.Real{}, "matt")

	dir := t.TempDir()
	write(t, dir, "bp.yaml", `records:
  - type: health.blood-pressure
    id: "2024-09-16 06:14:00:1"
    date_excel: "2024-09-16 06:14:00"
    reading: "1"
    systolic_blood_pressure: {value: 145, unit: mmHg}
    diastolic_blood_pressure: {value: 95, unit: mmHg}
    effective_time_frame: {date_time: "2024-09-16T06:14:00+10:00"}
    pulse: "70"
  - type: health.blood-pressure
    id: "wrong"
    date_excel: "2024-09-16 06:16:00"
    reading: "1"
    systolic_blood_pressure: {value: 142, unit: mmHg}
    diastolic_blood_pressure: {value: 88, unit: mmHg}
    effective_time_frame: {date_time: "2024-09-16T06:16:00+10:00"}
`)
	envs, err := records.Scan(dir, "matt", defs)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if want := facet.SourceRef("records", "2024-09-16 06:14:00", "1"); envs[0].SourceRef != want {
		t.Fatalf("composed ref = %q, want %q", envs[0].SourceRef, want)
	}
	for i, env := range envs {
		b, _ := g.Resolve(env)
		if _, _, err := g.Commit(ctx, env, b); err != nil {
			t.Fatalf("record %d refused: %v", i, err)
		}
	}
}

// A note about a facet is a records row like any other: it admits through the gate
// as its own facet, keyed on its own id, and the facet it is about is untouched. A
// re-run is a no-op; an edited note supersedes the note alone.
func TestRecords_ANoteAboutAFacetIsItsOwnFacet(t *testing.T) {
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	g := gate.New(l, defs, mvpScopes, clock.Real{}, "matt")

	dir := t.TempDir()
	const note = `extracted: {method: typed by the person, version: 1}
records:
  - type: note.annotation
    id: note:2c6f38a3:1
    at: 2026-10-09 11:29
    about: 2c6f38a3-3db7-5e65-b933-9c59980e1c6c
    about_version: 1
    text: During a large work meeting, presenting.
`
	write(t, dir, "notes.yaml", note)
	admit := func() []gate.Result {
		t.Helper()
		envs, err := records.Scan(dir, "matt", defs)
		if err != nil {
			t.Fatalf("Scan: %v", err)
		}
		var out []gate.Result
		for i, env := range envs {
			b, _ := g.Resolve(env)
			res, _, err := g.Commit(ctx, env, b)
			if err != nil {
				t.Fatalf("record %d refused: %v", i, err)
			}
			out = append(out, res)
		}
		return out
	}
	first := admit()
	if len(first) != 1 || first[0].Outcome != gate.OutcomeAdmitted {
		t.Fatalf("first run: %+v", first)
	}
	if first[0].FacetID == "2c6f38a3-3db7-5e65-b933-9c59980e1c6c" {
		t.Fatal("the note took the facet's own id")
	}
	if again := admit(); again[0].Outcome != gate.OutcomeNoOp {
		t.Fatalf("re-run: %+v", again)
	}
	write(t, dir, "notes.yaml", strings.Replace(note, "presenting.", "presenting to the board.", 1))
	if edited := admit(); edited[0].Outcome != gate.OutcomeSuperseded || edited[0].FacetID != first[0].FacetID {
		t.Fatalf("edited note: %+v, first %s", edited, first[0].FacetID)
	}
}
