package records_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"

	"github.com/mattjoyce/anam/internal/capture/records"
)

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

// A directory of records files scans to one envelope per record, source_ref from the
// id, payload without the type and without the file's provenance.
func TestScan_DirectoryToEnvelopes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "lipids.yaml", labFile)
	write(t, dir, "README.txt", "not a records file")

	envs, err := records.Scan(dir, "matt", bundled(t))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("got %d envelopes, want 2", len(envs))
	}
	e := envs[0]
	if e.Source != "records" || e.Injector != "records" || e.Type != "lab.result" {
		t.Errorf("envelope identity wrong: %+v", e)
	}
	if e.SourceRef != facet.SourceRef("records", "acl:39986323/total-chol") {
		t.Errorf("source_ref %q", e.SourceRef)
	}
	var payload map[string]any
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	for _, k := range []string{"type", "document", "extracted"} {
		if _, ok := payload[k]; ok {
			t.Errorf("payload carries %q; it belongs to the envelope or the file", k)
		}
	}
	if payload["id"] != "acl:39986323/total-chol" || payload["value"] != "5.0" {
		t.Errorf("payload fields wrong: %v", payload)
	}
}

func TestScan_RefusesBadFiles(t *testing.T) {
	cases := map[string]string{
		"duplicate id": labFile + "  - type: lab.result\n    id: acl:39986323/total-chol\n    collected: x\n",
		"bad type":     "records:\n  - type: nonsense\n    id: a\n",
		"no id":        "records:\n  - type: lab.result\n    value: 1\n",
		"unknown key":  "recordz:\n  - type: lab.result\n    id: a\n",
		"empty":        "records: []\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "f.yaml", body)
			_, err := records.Scan(dir, "matt", bundled(t))
			if !errors.Is(err, records.ErrBadRecords) {
				t.Fatalf("err = %v, want ErrBadRecords", err)
			}
		})
	}
}

// YAML's own guesses do not reach the payload: a date-like collected time stays the
// text that was typed, a numeric-looking id stays text, and a number stays a number.
func TestScan_ScalarsAsWritten(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "f.yaml", `records:
  - type: lab.result
    id: 20240916
    collected: 2024-09-10 07:46
    value_num: 5.7
    flagged: true
`)
	envs, err := records.Scan(dir, "matt", bundled(t))
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	var p map[string]any
	if err := json.Unmarshal(envs[0].Payload, &p); err != nil {
		t.Fatal(err)
	}
	if p["collected"] != "2024-09-10 07:46" || p["id"] != "20240916" || p["value_num"] != 5.7 || p["flagged"] != true {
		t.Fatalf("payload %v", p)
	}
	if envs[0].SourceRef != facet.SourceRef("records", "20240916") {
		t.Fatalf("source_ref %q", envs[0].SourceRef)
	}
}

// bundled is the real Definition set: identity composes as the gate composes it.
func bundled(t *testing.T) *def.Registry {
	t.Helper()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	return defs
}
