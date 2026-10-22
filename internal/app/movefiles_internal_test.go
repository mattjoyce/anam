package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// move-artifacts over a real ledger, two real operational stores and a folder written
// the way the artifact store wrote it: the receipts a store published are the folder's
// files, a second store takes them in, and they verify there.

// storeAt opens the operational store at path, as the tests need to read it.
func storeAt(t *testing.T, path string) *ops.DB {
	t.Helper()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(context.Background(), path, buckets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// exportFolder writes every receipt and served value db holds as the artifact store
// filed them: <kind>/sha256-<hex>.json.
func exportFolder(t *testing.T, db *ops.DB) (dir string, receipts []string) {
	t.Helper()
	ctx := context.Background()
	dir = t.TempDir()
	for _, kind := range []string{ops.KindReceipt, ops.KindServed} {
		digests, err := db.List(ctx, kind)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range digests {
			b, err := db.Get(ctx, kind, d)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(dir, kind), 0o750); err != nil {
				t.Fatal(err)
			}
			writeDoc(t, filepath.Join(dir, kind, "sha256-"+strings.TrimPrefix(d, "sha256:")+".json"), string(b))
			if kind == ops.KindReceipt {
				receipts = append(receipts, d)
			}
		}
	}
	return dir, receipts
}

func TestMoveArtifacts_ARecordsFolderComesInAndItsReceiptsVerify(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	f.admit(t, bpReadings)
	// A bundled projection serves the readings, and each serve files a receipt.
	for _, narrow := range [][]string{{}, {"--to", "2026-02-10"}} {
		if _, _, err := cli(t, f.cfgPath, append([]string{"timeline", "--projection", "bp", "--publish"}, narrow...)...); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
	old := storeAt(t, filepath.Join(f.dir, "tracking.db"))
	folder, receipts := exportFolder(t, old)
	if len(receipts) != 2 {
		t.Fatalf("exported %d receipts, want 2", len(receipts))
	}

	// The old store is set aside: a registered projection in it comes across with the
	// move, and the new store has no folder beside it.
	if err := old.RegisterProjection(context.Background(),
		ops.ProjectionRow{Name: "heart", Version: 1, Body: heartTemplate}, "2026-10-23T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(t.TempDir(), "fresh.db")
	cfg := filepath.Join(f.dir, "fresh.yaml")
	writeDoc(t, cfg, fmt.Sprintf("person_id: matt\nledger_path: %s\ndb_path: %s\n", f.ledgerPath, fresh))

	// Without --yes it reports and writes nothing.
	out, _, err := cli(t, cfg, "move-artifacts", "--from", folder)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	for _, want := range []string{"every file hashes to its name", "receipt", "served", "total", "nothing written"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report lacks %q:\n%s", want, out)
		}
	}
	if got, _ := storeAt(t, fresh).List(context.Background(), ops.KindReceipt); len(got) != 0 {
		t.Fatalf("the report wrote %d receipts", len(got))
	}

	out, _, err = cli(t, cfg, "move-artifacts", "--from", folder, "--registered-from", filepath.Join(f.dir, "tracking.db"), "--yes")
	if err != nil {
		t.Fatalf("move: %v", err)
	}
	// The old store already holds the two receipts and their served values as rows, so
	// the carry brings them, and the folder's copies are found already there.
	if !strings.Contains(out, "copied 0 registered Definitions, 1 registered projection templates, 0 inline projection bodies, "+
		"0 admission holds, 2 receipts, 2 served values, 0 saved selections and 0 selection codes") {
		t.Errorf("the move does not say what it copied:\n%s", out)
	}

	listed, _, err := cli(t, cfg, "receipt", "list")
	if err != nil {
		t.Fatalf("receipt list: %v", err)
	}
	for _, hash := range receipts {
		if !strings.Contains(listed, hash) {
			t.Errorf("receipt list lacks %s:\n%s", hash, listed)
		}
		verified, _, err := cli(t, cfg, "receipt", "verify", hash)
		if err != nil || !strings.Contains(verified, "OK   reproduced") || !strings.Contains(verified, "OK   intact") {
			t.Errorf("receipt %s after the move: %v\n%s", hash, err, verified)
		}
		if _, _, err := cli(t, cfg, "receipt", "show", hash); err != nil {
			t.Errorf("receipt show %s: %v", hash, err)
		}
	}
	rows, err := storeAt(t, fresh).RegisteredProjections(context.Background())
	if err != nil || len(rows) != 1 || rows[0].Name != "heart" {
		t.Errorf("the new store holds registered projections %v (%v), want heart", rows, err)
	}

	// Again: nothing is loaded twice.
	out, _, err = cli(t, cfg, "move-artifacts", "--from", folder, "--yes")
	if err != nil {
		t.Fatalf("second move: %v", err)
	}
	if !strings.Contains(out, "ALREADY THERE") {
		t.Errorf("the second move does not report the rows already there:\n%s", out)
	}
}

func TestMoveArtifacts_RefusesAFolderWithAnAlteredFile(t *testing.T) {
	t.Parallel()
	f := newLedgerFixture(t)
	f.admit(t, bpReadings)
	if _, _, err := cli(t, f.cfgPath, "timeline", "--projection", "bp", "--publish"); err != nil {
		t.Fatal(err)
	}
	folder, receipts := exportFolder(t, storeAt(t, filepath.Join(f.dir, "tracking.db")))
	name := filepath.Join(folder, ops.KindReceipt, "sha256-"+strings.TrimPrefix(receipts[0], "sha256:")+".json")
	raw, err := os.ReadFile(name) //nolint:gosec // test-owned temp path
	if err != nil {
		t.Fatal(err)
	}
	writeDoc(t, name, strings.Replace(string(raw), `"watermark"`, `"watermarks"`, 1))

	fresh := filepath.Join(t.TempDir(), "fresh.db")
	cfg := filepath.Join(f.dir, "fresh.yaml")
	writeDoc(t, cfg, fmt.Sprintf("person_id: matt\nledger_path: %s\ndb_path: %s\n", f.ledgerPath, fresh))
	_, _, err = cli(t, cfg, "move-artifacts", "--from", folder, "--yes")
	if err == nil || !strings.Contains(err.Error(), "do not match") {
		t.Fatalf("move over an altered file: %v, want a refusal", err)
	}
	if !strings.Contains(err.Error(), receipts[0][len("sha256:"):]) {
		t.Errorf("the refusal does not name the file: %v", err)
	}
	for _, kind := range ops.Kinds() {
		if got, _ := storeAt(t, fresh).List(context.Background(), kind); len(got) != 0 {
			t.Errorf("%d %s rows were written by a refused move", len(got), kind)
		}
	}
}
