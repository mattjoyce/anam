package ops_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// A real SQLite file per test (AGENTS.md): t.TempDir gives each its own.

// buckets is the bucketing these tests open at; the width-mismatch test is the one
// that opens at another.
func buckets(t *testing.T, w ...span.Width) span.Buckets {
	t.Helper()
	width := span.Default
	if len(w) == 1 {
		width = w[0]
	}
	b, err := span.New(width)
	if err != nil {
		t.Fatalf("span.New(%q): %v", width, err)
	}
	return b
}

func open(t *testing.T) (*ops.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "anam.db")
	d, err := ops.Open(context.Background(), path, buckets(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d, path
}

func TestOpen_IsIdempotent(t *testing.T) {
	t.Parallel()
	d, path := open(t)
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Re-opening an existing database must apply the schema as a no-op.
	d2, err := ops.Open(context.Background(), path, buckets(t))
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() { _ = d2.Close() }()
}

func TestOpen_CreatesParentDirectories(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "deep", "nested", "anam.db")
	d, err := ops.Open(context.Background(), path, buckets(t))
	if err != nil {
		t.Fatalf("Open with missing parents: %v", err)
	}
	defer func() { _ = d.Close() }()
}

func TestOpen_FailsFastOnAnUncreatableDir(t *testing.T) {
	t.Parallel()
	// A regular file where a parent directory must go: MkdirAll cannot succeed.
	obstruction := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(obstruction, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.Open(context.Background(), filepath.Join(obstruction, "anam.db"), buckets(t)); err == nil {
		t.Fatal("Open under a file-as-directory succeeded, want error")
	}
}

func TestOpen_RefusesForeignSchemaVersion(t *testing.T) {
	t.Parallel()
	d, path := open(t)

	// Simulate a database written by a future build.
	if _, err := d.SQL().Exec(`UPDATE meta SET value = '999' WHERE key = 'schema_version'`); err != nil {
		t.Fatalf("update version: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := ops.Open(context.Background(), path, buckets(t))
	if !errors.Is(err, ops.ErrSchemaVersion) {
		t.Fatalf("Open on v999 database = %v, want ErrSchemaVersion", err)
	}
}

// bucket_id is persisted and catch-up only rewrites the rows new events touch, so
// re-opening at another width would accrete two kinds of well-formed id in one index
// with nothing to tell them apart. Refuse instead: the derived tier replays.
func TestOpen_RefusesAnIndexBuiltAtAnotherBucketWidth(t *testing.T) {
	t.Parallel()
	d, path := open(t)
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err := ops.Open(context.Background(), path, buckets(t, span.Weekly))
	if !errors.Is(err, ops.ErrBucketWidth) {
		t.Fatalf("Open of a monthly index at weekly = %v, want ErrBucketWidth", err)
	}

	// The same width still opens: the pin refuses a change, not a re-open.
	again, err := ops.Open(context.Background(), path, buckets(t))
	if err != nil {
		t.Fatalf("re-Open at the pinned width: %v", err)
	}
	_ = again.Close()
}

// The head of a span is its latest checkpoint — a view over the log, so it cannot
// disagree with it. This is the W/Wv seam the mint (R8) will drive.
func TestSpanHead_IsTheLatestCheckpoint(t *testing.T) {
	t.Parallel()
	d, _ := open(t)

	insert := `INSERT INTO checkpoint (span_id, v, covers_w, value_hash, minted_at) VALUES (?, ?, ?, ?, ?)`
	rows := [][]any{
		{"bucket:2026-W27", 1, 40, "sha256:aaa", "2026-07-06T00:00:00.000000000Z"},
		{"bucket:2026-W27", 2, 58, "sha256:bbb", "2026-07-09T00:00:00.000000000Z"},
		{"bucket:2026-W28", 1, 55, "sha256:ccc", "2026-07-09T00:00:00.000000000Z"},
	}
	for _, r := range rows {
		if _, err := d.SQL().Exec(insert, r...); err != nil {
			t.Fatalf("insert checkpoint: %v", err)
		}
	}

	var v, coversW int
	var hash string
	err := d.SQL().QueryRow(
		`SELECT v, covers_w, value_hash FROM span_head WHERE span_id = 'bucket:2026-W27'`).
		Scan(&v, &coversW, &hash)
	if err != nil {
		t.Fatalf("query span_head: %v", err)
	}
	if v != 2 || coversW != 58 || hash != "sha256:bbb" {
		t.Errorf("span_head(W27) = v%d covers_w=%d %s, want v2 covers_w=58 sha256:bbb", v, coversW, hash)
	}

	var heads int
	if err := d.SQL().QueryRow(`SELECT COUNT(*) FROM span_head`).Scan(&heads); err != nil {
		t.Fatalf("count heads: %v", err)
	}
	if heads != 2 {
		t.Errorf("span_head rows = %d, want 2 (one per span)", heads)
	}
}

// The replay cursor is a singleton: row id must be 1.
func TestReplayCursor_IsASingleton(t *testing.T) {
	t.Parallel()
	d, _ := open(t)

	if _, err := d.SQL().Exec(`INSERT INTO replay_cursor (id, last_seq) VALUES (1, 42)`); err != nil {
		t.Fatalf("insert singleton: %v", err)
	}
	if _, err := d.SQL().Exec(`INSERT INTO replay_cursor (id, last_seq) VALUES (2, 99)`); err == nil {
		t.Error("a second replay_cursor row was accepted; the CHECK (id = 1) is not holding")
	}
}

// Losing the file is a replay, not a fact: a fresh Open on a fresh path must yield a
// complete, empty, usable schema. This is the executable meaning of the derived tier.
func TestLostFile_ReopensEmptyAndUsable(t *testing.T) {
	t.Parallel()
	d, path := open(t)

	if _, err := d.SQL().Exec(
		`INSERT INTO facet_index (facet_id, type, fact_time, bucket_id, current_version, status, content_hash)
		 VALUES ('f1', 'health.blood-pressure', '2026-07-05T06:30:00.000000000Z', 'bucket:2026-W27', 1, 'admitted', 'sha256:aaa')`); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove: %v", err)
	}

	d2, err := ops.Open(context.Background(), path, buckets(t))
	if err != nil {
		t.Fatalf("re-Open after loss: %v", err)
	}
	defer func() { _ = d2.Close() }()

	var n int
	if err := d2.SQL().QueryRow(`SELECT COUNT(*) FROM facet_index`).Scan(&n); err != nil {
		t.Fatalf("query rebuilt schema: %v", err)
	}
	if n != 0 {
		t.Errorf("fresh database holds %d facet rows, want 0", n)
	}
}

// The derived tier names the ledger it was derived from. The cursor is only a number,
// and a number cannot tell this ledger's events from another's.
func TestPinLedger_RefusesADifferentLedger(t *testing.T) {
	t.Parallel()
	d, _ := open(t)
	ctx := context.Background()

	if err := d.PinLedger(ctx, "ledger-a"); err != nil {
		t.Fatalf("first pin: %v", err)
	}
	if err := d.PinLedger(ctx, "ledger-a"); err != nil {
		t.Errorf("re-pinning the same ledger = %v, want nil", err)
	}
	if err := d.PinLedger(ctx, "ledger-b"); !errors.Is(err, ops.ErrLedgerChanged) {
		t.Errorf("pinning another ledger = %v, want ErrLedgerChanged", err)
	}
	if err := d.PinLedger(ctx, ""); err == nil {
		t.Error("an empty ledger id was pinned")
	}
}
