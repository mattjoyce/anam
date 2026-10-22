package backup_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/backup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// A backup over a real ledger, operational store, artifact tier and library, each in
// the test's own temp directory.
func TestTake_RestoresAsTheRecordAndKeepsTheNewest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	data := t.TempDir()
	l, err := ledger.Create(ctx, filepath.Join(data, "ledger.db"), "matt")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, filepath.Join(data, "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	id, err := l.ID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.PinLedger(ctx, id); err != nil {
		t.Fatal(err)
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A receipt is a row of the operational store, so the backup of the store carries it.
	receiptHash, err := db.Put(ctx, ops.KindReceipt, []byte(`{"served_hash":"sha256:x","watermark":0}`))
	if err != nil {
		t.Fatal(err)
	}
	write(filepath.Join(data, "documents", "2024", "letter.md"), "# a letter")

	dir := filepath.Join(t.TempDir(), "backups")
	write(filepath.Join(dir, "notes.txt"), "not a backup")                      // never touched
	write(filepath.Join(dir, "anam-20261001T000000Z.partial", "ledger.db"), "") // a crash's leftover

	at := time.Date(2026, 10, 9, 23, 0, 0, 0, time.UTC)
	d := backup.Deps{
		Ledger: l, Ops: db,
		LibraryDir: filepath.Join(data, "documents"), Dir: dir, Keep: 2,
	}
	var last string
	for i := 0; i < 3; i++ {
		d.Clock = clock.Fixed{Instant: at.Add(time.Duration(i) * 24 * time.Hour)}
		b, err := backup.Take(ctx, d)
		if err != nil {
			t.Fatalf("Take %d: %v", i, err)
		}
		if b.Files != 3 || b.W != 0 {
			t.Fatalf("Take %d: %+v, want 3 files at W=0", i, b)
		}
		if i == 2 && (len(b.Pruned) != 1 || b.Pruned[0] != "anam-20261009T230000Z") {
			t.Fatalf("third backup pruned %v, want the first", b.Pruned)
		}
		last = b.Path
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	want := []string{"anam-20261010T230000Z", "anam-20261011T230000Z", "notes.txt"}
	if len(names) != len(want) {
		t.Fatalf("backup dir holds %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("backup dir holds %v, want %v", names, want)
		}
	}

	// The newest restores: the ledger opens as this ledger, the store names it, and
	// the files are there.
	rl, err := ledger.Open(ctx, filepath.Join(last, "ledger.db"), "matt")
	if err != nil {
		t.Fatalf("open the backed-up ledger: %v", err)
	}
	defer func() { _ = rl.Close() }()
	if got, _ := rl.ID(ctx); got != id {
		t.Fatalf("backup ledger %q, want %q", got, id)
	}
	rdb, err := ops.Open(ctx, filepath.Join(last, "tracking.db"), buckets)
	if err != nil {
		t.Fatalf("open the backed-up store: %v", err)
	}
	defer func() { _ = rdb.Close() }()
	if got, _ := rdb.LedgerID(ctx); got != id {
		t.Fatalf("backup store pins %q, want %q", got, id)
	}
	if _, err := rdb.Get(ctx, ops.KindReceipt, receiptHash); err != nil {
		t.Fatalf("the backed-up store does not hold the receipt: %v", err)
	}
	for _, f := range []string{"documents/2024/letter.md"} {
		if _, err := os.Stat(filepath.Join(last, f)); err != nil {
			t.Errorf("backup lacks %s: %v", f, err)
		}
	}
}

// Without a directory the daemon takes no backup, and says why.
func TestTake_RefusesWithoutADirectory(t *testing.T) {
	t.Parallel()
	if _, err := backup.Take(context.Background(), backup.Deps{Keep: 1}); err == nil {
		t.Fatal("Take with no directory succeeded")
	}
}
