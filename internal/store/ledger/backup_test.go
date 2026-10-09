package ledger_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

// A backup is a ledger: it opens as one, names the same ledger, holds the same W and
// facts, and it refuses to write over a file.
func TestBackupTo_IsTheSameLedger(t *testing.T) {
	t.Parallel()
	l := newLedger(t)
	ctx := context.Background()
	f := testFacet(t, "bp-csv:2026-01-24T06:30:00", `{"systolic": 118}`, 1)
	if err := l.Admit(ctx, 0, f, admittedEvent(f, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	path := filepath.Join(t.TempDir(), "backup", "ledger.db")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := l.BackupTo(ctx, path); err != nil {
		t.Fatalf("BackupTo: %v", err)
	}
	if err := l.BackupTo(ctx, path); err == nil {
		t.Fatal("BackupTo wrote over an existing file")
	}

	b, err := ledger.Open(ctx, path, person)
	if err != nil {
		t.Fatalf("open the backup: %v", err)
	}
	defer func() { _ = b.Close() }()
	idA, _ := l.ID(ctx)
	idB, err := b.ID(ctx)
	if err != nil || idA != idB {
		t.Fatalf("backup names %q (%v), ledger %q", idB, err, idA)
	}
	if w, err := b.Head(ctx); err != nil || w != 1 {
		t.Fatalf("backup W = %d (%v), want 1", w, err)
	}
	if got, err := b.CurrentVersion(ctx, f.ID); err != nil || string(got.Payload.Bytes()) != string(f.Payload.Bytes()) {
		t.Fatalf("backup facet: %v", err)
	}
}
