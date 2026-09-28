package ledger_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite" // the tests build non-ledger files to point the ledger at

	"github.com/mattjoyce/anam/internal/store/ledger"
)

// A mistyped path must fail. It must not start an empty record beside the real one.
func TestOpen_MissingFileIsNotInitialisedAndCreatesNothing(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nowhere", "ledger.db")

	for name, open := range map[string]func(context.Context, string, string) (*ledger.Ledger, error){
		"Open": ledger.Open, "OpenReadOnly": ledger.OpenReadOnly,
	} {
		if l, err := open(ctx, path, person); !errors.Is(err, ledger.ErrNotInitialised) {
			if l != nil {
				_ = l.Close()
			}
			t.Errorf("%s on a missing file = %v, want ErrNotInitialised", name, err)
		}
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("opening a missing ledger created something at %s", filepath.Dir(path))
	}
}

// The person is pinned in the file. A wrong person_id fails loudly on every way in.
func TestOpen_RefusesADifferentPerson(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := newLedger(t).Path()

	if l, err := ledger.Open(ctx, path, "someone-else"); !errors.Is(err, ledger.ErrPersonMismatch) {
		closeIfOpen(l)
		t.Errorf("Open as another person = %v, want ErrPersonMismatch", err)
	}
	if l, err := ledger.OpenReadOnly(ctx, path, "someone-else"); !errors.Is(err, ledger.ErrPersonMismatch) {
		closeIfOpen(l)
		t.Errorf("OpenReadOnly as another person = %v, want ErrPersonMismatch", err)
	}
	if l, err := ledger.Create(ctx, path, "someone-else"); !errors.Is(err, ledger.ErrPersonMismatch) {
		closeIfOpen(l)
		t.Errorf("Create as another person = %v, want ErrPersonMismatch", err)
	}
}

// The likeliest wrong file is the operational store, reached through a mistyped
// ledger_path. Init must refuse it and leave nothing of the ledger inside it.
func TestCreate_RefusesAFileThatIsNotALedger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "tracking.db")

	other := rawDB(t, path)
	if _, err := other.ExecContext(ctx, `CREATE TABLE facet_index (facet_id TEXT PRIMARY KEY)`); err != nil {
		t.Fatalf("build the foreign file: %v", err)
	}

	if l, err := ledger.Create(ctx, path, person); !errors.Is(err, ledger.ErrSchemaMismatch) {
		closeIfOpen(l)
		t.Fatalf("Create over a foreign file = %v, want ErrSchemaMismatch", err)
	}

	var n int
	err := other.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master
		WHERE name IN ('facet', 'anam_event', 'ledger_meta')`).Scan(&n)
	if err != nil {
		t.Fatalf("inspect the foreign file: %v", err)
	}
	if n != 0 {
		t.Errorf("the refused Create left %d ledger objects in the foreign file", n)
	}
}

// A ledger written by another schema version is refused, and the error must not
// suggest deleting it. That advice is right for the operational store and wrong here.
func TestOpen_RefusesAnotherSchemaVersion(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "ledger.db")

	old := rawDB(t, path)
	if _, err := old.ExecContext(ctx, `
		CREATE TABLE ledger_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
		INSERT INTO ledger_meta VALUES ('schema_version', '0'), ('person_id', 'test-person'), ('ledger_id', 'x');
	`); err != nil {
		t.Fatalf("build the old ledger: %v", err)
	}

	l, err := ledger.Open(ctx, path, person)
	closeIfOpen(l)
	if !errors.Is(err, ledger.ErrSchemaMismatch) {
		t.Fatalf("Open on schema 0 = %v, want ErrSchemaMismatch", err)
	}
}

// A reader's handle cannot write. This is what opening the ledger file separately
// from the operational store buys: the refusal is SQLite's, not a convention.
func TestOpenReadOnly_RefusesEveryWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newLedger(t)
	f := testFacet(t, "bp-csv:2026-05-01T06:30:00", `{"systolic": 118}`, 1)
	if err := w.Admit(ctx, 0, f, admittedEvent(f, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	r := openReader(t, w.Path())

	got, err := r.Version(ctx, f.ID, 1)
	if err != nil || got.Descriptor.ContentHash != f.Descriptor.ContentHash {
		t.Fatalf("a reader could not read: %v", err)
	}

	f2 := testFacet(t, "bp-csv:2026-05-02T06:30:00", `{"systolic": 120}`, 1)
	if err := r.Admit(ctx, 1, f2, admittedEvent(f2, 2)); err == nil {
		t.Error("a read-only handle admitted a facet")
	}
	if err := r.Mark(ctx, 1, admittedEvent(f, 2)); err == nil {
		t.Error("a read-only handle marked an event")
	}
	if err := r.Drop(ctx); err == nil {
		t.Error("a read-only handle dropped the ledger")
	}
	if head, err := w.Head(ctx); err != nil || head != 1 {
		t.Errorf("head = %d (err %v) after the refused writes, want 1", head, err)
	}
}

// The inspector reads while ingest writes, from another connection. A reader must
// neither wait for an open write nor see any of it before it commits.
func TestOpenReadOnly_IsNotBlockedByAnOpenWrite(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newLedger(t)
	r := openReader(t, w.Path())

	// Hold the write lock the way an ingest does mid-transaction.
	writer := rawDB(t, w.Path())
	conn, err := writer.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		t.Fatalf("take the write lock: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO anam_event
		(seq, kind, facet_id, anam_time, artifact_version) VALUES (1, 'admitted', 'x', 'x', 1)`); err != nil {
		t.Fatalf("write under the lock: %v", err)
	}

	readCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if head, err := r.Head(readCtx); err != nil || head != 0 {
		t.Errorf("head during an open write = %d (err %v), want 0 and no wait", head, err)
	}

	if _, err := conn.ExecContext(ctx, `COMMIT`); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if head, err := r.Head(ctx); err != nil || head != 1 {
		t.Errorf("head after the commit = %d (err %v), want 1", head, err)
	}
}

// When a writer dies, its committed transactions are in the write-ahead log and the
// shared index beside it is gone or stale. A reader arriving afterwards must recover
// them. The crash is staged by copying the file and its log out from under a live
// writer, which is what a killed process leaves behind.
func TestOpenReadOnly_RecoversWhatADeadWriterCommitted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	w := newLedger(t)

	cs := batchOf(t, 0, 5, "crash")
	if err := w.AdmitBatch(ctx, 0, cs); err != nil {
		t.Fatalf("AdmitBatch: %v", err)
	}

	wreck := filepath.Join(t.TempDir(), "ledger.db")
	copyFile(t, w.Path(), wreck)
	copyFile(t, w.Path()+"-wal", wreck+"-wal")

	r := openReader(t, wreck)
	if head, err := r.Head(ctx); err != nil || head != 5 {
		t.Fatalf("head of the recovered ledger = %d (err %v), want 5", head, err)
	}
	for _, c := range cs {
		got, err := r.Version(ctx, c.Facet.ID, 1)
		if err != nil {
			t.Errorf("%s: %v", c.Facet.Provenance.SourceRef, err)
			continue
		}
		if string(got.Payload.Bytes()) != string(c.Facet.Payload.Bytes()) {
			t.Errorf("%s: payload did not survive the crash", c.Facet.Provenance.SourceRef)
		}
	}
}

// A rebuilt ledger is a different ledger, and says so. The operational store pins the
// id it was derived from.
func TestDrop_ThenInitIsANewLedger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l := newLedger(t)

	before, err := l.ID(ctx)
	if err != nil {
		t.Fatalf("ID: %v", err)
	}
	f := testFacet(t, "bp-csv:2026-06-01T06:30:00", `{"systolic": 118}`, 1)
	if err := l.Admit(ctx, 0, f, admittedEvent(f, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}

	if err := l.Drop(ctx); err != nil {
		t.Fatalf("Drop: %v", err)
	}
	if _, err := l.Head(ctx); !errors.Is(err, ledger.ErrNotInitialised) {
		t.Errorf("Head on a dropped ledger = %v, want ErrNotInitialised", err)
	}
	if err := l.Init(ctx); err != nil {
		t.Fatalf("re-Init: %v", err)
	}

	after, err := l.ID(ctx)
	if err != nil {
		t.Fatalf("ID after rebuild: %v", err)
	}
	if after == before {
		t.Error("the rebuilt ledger kept the old ledger's id")
	}
	if head, err := l.Head(ctx); err != nil || head != 0 {
		t.Errorf("head of the rebuilt ledger = %d (err %v), want 0", head, err)
	}
}

// A path is config, and config can hold characters a URI treats as structure.
func TestCreate_PathWithURICharacters(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "odd ?#% name", "ledger.db")

	l, err := ledger.Create(ctx, path, person)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() { _ = l.Close() }()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("the ledger is not at the path it was given: %v", err)
	}
}

func openReader(t *testing.T, path string) *ledger.Ledger {
	t.Helper()
	r, err := ledger.OpenReadOnly(context.Background(), path, person)
	if err != nil {
		t.Fatalf("OpenReadOnly: %v", err)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}

func rawDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func closeIfOpen(l *ledger.Ledger) {
	if l != nil {
		_ = l.Close()
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	src, err := os.Open(from) //nolint:gosec // test path under t.TempDir
	if err != nil {
		t.Fatalf("open %s: %v", from, err)
	}
	defer func() { _ = src.Close() }()
	dst, err := os.Create(to) //nolint:gosec // test path under t.TempDir
	if err != nil {
		t.Fatalf("create %s: %v", to, err)
	}
	defer func() { _ = dst.Close() }()
	if _, err := io.Copy(dst, src); err != nil {
		t.Fatalf("copy %s: %v", from, err)
	}
}
