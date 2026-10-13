package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Three readings in the blood pressure export's own format.
// bpFixture is a blood pressure diary read into a records file: three readings, ids
// in the diary's form (date_excel and reading joined by a colon). The phone app's
// exports have their own importer since card 80, so the ingest tests use the records
// source, which still admits blood pressure. "114.0" appears once, so a test can
// correct one reading by replacing it.
const bpFixture = `document: {name: bp-diary.pdf, sha256: "0000000000000000000000000000000000000000000000000000000000000000"}
extracted: {method: by hand, version: 1}
records:
  - {type: health.blood-pressure, id: "2026-1-24 06:36:00:1", date_excel: "2026-1-24 06:36:00", reading: "1", systolic_blood_pressure: {value: 114.0, unit: mmHg}, diastolic_blood_pressure: {value: 80.0, unit: mmHg}, effective_time_frame: {date_time: "2026-01-24T06:36:00+11:00"}, pulse: "70.0"}
  - {type: health.blood-pressure, id: "2026-2-10 05:04:00:1", date_excel: "2026-2-10 05:04:00", reading: "1", systolic_blood_pressure: {value: 102.0, unit: mmHg}, diastolic_blood_pressure: {value: 72.0, unit: mmHg}, effective_time_frame: {date_time: "2026-02-10T05:04:00+11:00"}, pulse: "68.0"}
  - {type: health.blood-pressure, id: "2026-2-11 05:10:00:1", date_excel: "2026-2-11 05:10:00", reading: "1", systolic_blood_pressure: {value: 101.0, unit: mmHg}, diastolic_blood_pressure: {value: 72.0, unit: mmHg}, effective_time_frame: {date_time: "2026-02-11T05:10:00+11:00"}, pulse: "66.0"}
`

// ingestFixture is a real ledger, a real config and a readable records file of blood
// pressure readings, all under the test's own directory. bpPath is the file;
// recordsDir is the folder a records import names.
type ingestFixture struct {
	dir, cfgPath, ledgerPath, bpPath, recordsDir string
}

func newIngestFixture(t *testing.T) ingestFixture {
	t.Helper()
	dir := t.TempDir()
	f := ingestFixture{
		dir:        dir,
		cfgPath:    filepath.Join(dir, "config.yaml"),
		ledgerPath: filepath.Join(dir, "ledger.db"),
		bpPath:     filepath.Join(dir, "records", "bp-diary.yaml"),
		recordsDir: filepath.Join(dir, "records"),
	}
	if err := os.MkdirAll(f.recordsDir, 0o750); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf("person_id: matt\nledger_path: %s\ndb_path: %s\nartifact_dir: %s\n",
		f.ledgerPath, filepath.Join(dir, "tracking.db"), filepath.Join(dir, "artifacts"))
	if err := os.WriteFile(f.cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.bpPath, []byte(bpFixture), 0o600); err != nil {
		t.Fatal(err)
	}
	l, err := ledger.Create(context.Background(), f.ledgerPath, "matt")
	if err != nil {
		t.Fatalf("create ledger: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

// ingest runs the command against the fixture with the given declarations and returns
// its output, the ledger's head afterwards, and the command's error.
func (f ingestFixture) ingest(t *testing.T, imports string) (string, ledger.Watermark, error) {
	t.Helper()
	importsPath := filepath.Join(f.dir, "imports.yaml")
	if err := os.WriteFile(importsPath, []byte(imports), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := ingestCmd(&f.cfgPath)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--imports", importsPath})
	cmd.SilenceUsage = true
	runErr := cmd.ExecuteContext(context.Background())

	l, err := ledger.OpenReadOnly(context.Background(), f.ledgerPath, "matt")
	if err != nil {
		t.Fatalf("reopen ledger: %v", err)
	}
	defer func() { _ = l.Close() }()
	w, err := l.Head(context.Background())
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	return out.String(), w, runErr
}

// A scheduled ingest reads two of its three sources over a network share. When the
// share is unmounted those paths do not resolve, and the run must stop before it
// writes: a ledger that holds one source's new rows and not the others' is a state no
// complete run produces.
func TestIngest_AnUnreadableSourceStopsTheRunBeforeAnythingIsAdmitted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		imports     func(f ingestFixture) string
		wantErr     string
		wantWritten bool
		wantOut     string
	}{
		{
			name: "every source readable",
			imports: func(f ingestFixture) string {
				return fmt.Sprintf("imports:\n  - source: records\n    path: %s\n", f.recordsDir)
			},
			wantWritten: true,
			wantOut:     "ledger changed: 3 facets admitted or superseded",
		},
		{
			name: "a later source is missing",
			imports: func(f ingestFixture) string {
				return fmt.Sprintf("imports:\n  - source: records\n    path: %s\n"+
					"  - source: documents\n    path: %s\n",
					f.recordsDir, filepath.Join(f.dir, "unmounted", "documents"))
			},
			wantErr: "documents",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newIngestFixture(t)
			out, w, err := f.ingest(t, tt.imports(f))

			if tt.wantErr == "" && err != nil {
				t.Fatalf("ingest: %v\n%s", err, out)
			}
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("ingest succeeded, want an error naming %q\n%s", tt.wantErr, out)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error %q does not name %q", err, tt.wantErr)
				}
			}
			if written := w > 0; written != tt.wantWritten {
				t.Errorf("ledger head is %d, want written=%v\n%s", w, tt.wantWritten, out)
			}
			if !strings.Contains(out, tt.wantOut) {
				t.Errorf("output does not say %q\n%s", tt.wantOut, out)
			}
		})
	}
}

// A scheduled run is quiet when nothing moved, so the line that says so is one a
// scheduler can match. The second of two identical runs must print it, and must leave
// the ledger where the first run did.
func TestIngest_ARepeatedRunSaysTheLedgerIsUnchanged(t *testing.T) {
	t.Parallel()
	f := newIngestFixture(t)
	imports := fmt.Sprintf("imports:\n  - source: records\n    path: %s\n", f.recordsDir)

	_, first, err := f.ingest(t, imports)
	if err != nil {
		t.Fatalf("first ingest: %v", err)
	}
	out, second, err := f.ingest(t, imports)
	if err != nil {
		t.Fatalf("second ingest: %v\n%s", err, out)
	}

	if second != first {
		t.Errorf("ledger head moved from %d to %d on a repeat run", first, second)
	}
	if !strings.Contains(out, nothingNew) {
		t.Errorf("output does not say %q\n%s", nothingNew, out)
	}
}
