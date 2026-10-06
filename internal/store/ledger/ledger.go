// Package ledger is the append-only record of what Anam knew (ARCHITECTURE.md §7, §8),
// held in one SQLite file of its own.
//
// This is the error kernel's substrate. Admit and Mark write an artifact and/or an
// anam-event in one transaction, so a caller may crash at any moment and re-run to
// known-good: the transaction either happened or it did not, and re-running an attempt
// that happened reports ErrAlreadyWritten instead of writing twice.
//
// The ledger file is separate from the operational store for three reasons. The
// operational store's repair is deletion, and that repair must never reach a fact. A
// reader can open this file read-only, so SQLite refuses its writes. And losing the
// operational file then proves the derived tier is derived, because nothing of the
// ledger could have been in it.
//
// Append-only is enforced by the database. Triggers refuse every UPDATE and DELETE and
// every overwriting INSERT, whoever issues them (schema.go). No method here mutates or
// removes a row.
//
// The ledger serves two access paths: the versions of one facet, and the anam-events
// since a watermark. Everything else a reader wants (the timeline, a type filter,
// changes_since) is an index held by the operational store and rebuilt by replay.
package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
)

// Watermark is a position in the anam-event stream. W is the seq of the most
// recent anam-event; genesis is 0 and seq 0 never exists.
//
// It is a distinct type, not a bare int64, so that a watermark and a checkpoint
// version can never be swapped in a comparison without the compiler objecting.
type Watermark int64

// The typed outcomes of a write or an open. Each is a distinct situation with a
// distinct recovery, and collapsing any two is the bug (§8).
var (
	// ErrAlreadyWritten: this exact artifact version already exists with the same
	// content hash. The previous attempt succeeded; treat as success.
	ErrAlreadyWritten = errors.New("ledger: artifact already written (idempotent no-op)")

	// ErrDivergentArtifact: this artifact version exists with a different content
	// hash. Someone wrote different bytes at the same identity. Stop.
	ErrDivergentArtifact = errors.New("ledger: artifact version exists with different content")

	// ErrStaleWatermark: the head is not the watermark the caller read. Re-read the
	// head, re-stamp anam_time, and retry. The retry loop belongs to the caller, the
	// gate, never to this package.
	ErrStaleWatermark = errors.New("ledger: watermark advanced since read")

	// ErrCorrupt: the database refused a write that this package had already checked
	// was sound. The checks and the constraints disagree, so no write can be trusted.
	ErrCorrupt = errors.New("ledger: corrupt: a checked write was refused by the database")

	// ErrBusy: another writer held the file's write lock for longer than the busy
	// timeout. Nothing was written. Re-run when the other writer has finished.
	ErrBusy = errors.New("ledger: locked by another writer")

	// ErrNotInitialised: there is no ledger at the path. Run `anam init`.
	ErrNotInitialised = errors.New("ledger: not initialised (run `anam init`)")

	// ErrSchemaMismatch: the file is not a ledger, or is a ledger written by a
	// different schema version. The remedy is a written migration or a fresh ledger at
	// a new path. It is never deletion: this file is the record.
	ErrSchemaMismatch = errors.New("ledger: file holds a different schema")

	// ErrPersonMismatch: the ledger belongs to a different person than the one
	// configured. A wrong person_id must fail loudly, not write into someone else's
	// timeline.
	ErrPersonMismatch = errors.New("ledger: file belongs to a different person")
)

// Ledger is a handle on one person's timeline in one file. Every method takes the
// caller's context and inherits its deadline: the store never sets its own (§8).
type Ledger struct {
	db     *sql.DB
	path   string
	person string
}

// The open modes, as SQLite's URI `mode` parameter spells them.
const (
	modeReadOnly  = "ro"
	modeReadWrite = "rw"
	modeCreate    = "rwc"
)

// busyTimeoutMS is how long a connection waits on the file's write lock before
// reporting ErrBusy. It sits below the default call timeout (config: 5s) so that
// contention is reported as ErrBusy and not as the caller's deadline expiring, which
// names nothing. The caller's context still bounds the wait from above.
const busyTimeoutMS = 3000

// Create opens the ledger at path for writing, creating the file and its parent
// directories if absent, and initialises it for personID. It is idempotent: on an
// existing ledger it verifies the pins and changes nothing.
func Create(ctx context.Context, path, personID string) (*Ledger, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("ledger: create data dir: %w", err)
		}
	}
	l, err := open(path, personID, modeCreate)
	if err != nil {
		return nil, err
	}
	if err := l.Init(ctx); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

// Open opens an existing ledger for writing. It creates nothing: a missing file is
// ErrNotInitialised, so a mistyped path fails instead of starting an empty record.
func Open(ctx context.Context, path, personID string) (*Ledger, error) {
	return openExisting(ctx, path, personID, modeReadWrite)
}

// OpenReadOnly opens an existing ledger for reading. SQLite refuses every write on the
// handle, so a reader cannot alter the record even by mistake. Every command that only
// reads the ledger opens it this way.
func OpenReadOnly(ctx context.Context, path, personID string) (*Ledger, error) {
	return openExisting(ctx, path, personID, modeReadOnly)
}

func openExisting(ctx context.Context, path, personID, mode string) (*Ledger, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: no file at %s", ErrNotInitialised, path)
		}
		return nil, fmt.Errorf("ledger: stat %q: %w", path, err)
	}
	l, err := open(path, personID, mode)
	if err != nil {
		return nil, err
	}
	if err := l.verify(ctx); err != nil {
		_ = l.Close()
		return nil, err
	}
	return l, nil
}

func open(path, personID, mode string) (*Ledger, error) {
	if personID == "" {
		return nil, fmt.Errorf("ledger: person id is required")
	}
	db, err := sql.Open("sqlite", dsn(path, mode))
	if err != nil {
		return nil, fmt.Errorf("ledger: open %q: %w", path, err)
	}
	return &Ledger{db: db, path: path, person: personID}, nil
}

// dsn builds the connection string for one open mode.
//
// A writer runs in WAL mode so that readers in other processes are never blocked by an
// ingest, with synchronous=FULL so that a commit the caller was told about survives
// power loss. Its transactions take the write lock at BEGIN (_txlock=immediate): the
// head read and the inserts that depend on it then happen under one lock, and no
// transaction can discover halfway that its snapshot went stale.
//
// A reader sets neither. The journal mode is a property of the file, and a read-only
// connection cannot change it.
func dsn(path, mode string) string {
	// The path sits in a URI, so the three characters that would end it are escaped.
	escaped := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(path)
	q := "mode=" + mode + "&_pragma=busy_timeout(" + strconv.Itoa(busyTimeoutMS) + ")"
	if mode != modeReadOnly {
		q += "&_pragma=journal_mode(WAL)&_pragma=synchronous(FULL)&_txlock=immediate"
	}
	return "file:" + escaped + "?" + q
}

// Init idempotently prepares the ledger: it applies the schema and pins the schema
// version, the person, and a fresh ledger id. A ledger written by an earlier schema
// is migrated forward here, step by step (migrate.go); this is the one place a
// ledger's schema moves, and it only ever adds.
//
// A file that already holds tables but no ledger_meta is somebody else's database,
// most plausibly the operational store reached through a mistyped path. Init refuses
// it with ErrSchemaMismatch and creates nothing in it.
func (l *Ledger) Init(ctx context.Context) error {
	from, err := l.initPins(ctx)
	if err != nil {
		return err
	}
	return l.migrate(ctx, from)
}

// initPins applies the base schema and the pins in one transaction and returns the
// file's effective schema version as it stood after them.
func (l *Ledger) initPins(ctx context.Context) (int, error) {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, classify("begin init", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	foreign, err := holdsForeignTables(ctx, tx)
	if err != nil {
		return 0, err
	}
	if foreign {
		return 0, fmt.Errorf("%w: %s holds tables but is not a ledger", ErrSchemaMismatch, l.path)
	}
	if _, err := tx.ExecContext(ctx, schema); err != nil {
		return 0, classify("apply schema", err)
	}

	for _, p := range []struct{ key, want string }{
		{metaSchemaVersion, strconv.Itoa(schemaVersion)},
		{metaPersonID, l.person},
		{metaLedgerID, uuid.NewString()},
	} {
		stored, err := pin(ctx, tx, p.key, p.want)
		if err != nil {
			return 0, err
		}
		if err := checkPin(p.key, stored, p.want); err != nil {
			return 0, err
		}
	}
	pins, err := readPins(ctx, tx)
	if err != nil {
		return 0, err
	}
	have, err := effectiveVersion(pins)
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %v", ErrSchemaMismatch, l.path, err)
	}
	if err := tx.Commit(); err != nil {
		return 0, classify("commit init", err)
	}
	return have, nil
}

// holdsForeignTables reports whether the file has tables of its own but no
// ledger_meta.
func holdsForeignTables(ctx context.Context, tx *sql.Tx) (bool, error) {
	var tables, meta int
	err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(name = 'ledger_meta'), 0)
		FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`).Scan(&tables, &meta)
	if err != nil {
		return false, classify("inspect file", err)
	}
	return tables > 0 && meta == 0, nil
}

// pin writes key unless it is already pinned, and returns what is stored. It reads
// before it writes because the insert trigger refuses an existing key outright.
func pin(ctx context.Context, tx *sql.Tx, key, value string) (string, error) {
	var stored string
	err := tx.QueryRowContext(ctx, `SELECT value FROM ledger_meta WHERE key = ?`, key).Scan(&stored)
	switch {
	case err == nil:
		return stored, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", classify("read pin "+key, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO ledger_meta (key, value) VALUES (?, ?)`, key, value); err != nil {
		return "", classify("write pin "+key, err)
	}
	return value, nil
}

// checkPin compares a stored pin against what this build and this config expect. The
// ledger id is generated, so whatever is stored is right, and the schema version is
// judged by effectiveVersion, after the migration records are counted.
func checkPin(key, stored, want string) error {
	if key == metaPersonID && stored != want {
		return fmt.Errorf("%w: ledger is pinned to %q, config says %q", ErrPersonMismatch, stored, want)
	}
	return nil
}

// verify checks an opened file is an initialised ledger for this person and this
// schema. It writes nothing, so it runs on a read-only handle too.
func (l *Ledger) verify(ctx context.Context) error {
	pins, err := readPins(ctx, l.db)
	if err != nil {
		if isNoSuchTable(err) {
			return fmt.Errorf("%w: %s is not a ledger", ErrNotInitialised, l.path)
		}
		return classify("read pins", err)
	}

	for _, p := range []struct{ key, want string }{
		{metaSchemaVersion, ""},
		{metaPersonID, l.person},
		{metaLedgerID, ""},
	} {
		stored, ok := pins[p.key]
		if !ok {
			return fmt.Errorf("%w: %s has no %s pin", ErrNotInitialised, l.path, p.key)
		}
		if err := checkPin(p.key, stored, p.want); err != nil {
			return err
		}
	}
	have, err := effectiveVersion(pins)
	if err != nil {
		return fmt.Errorf("%w: %s: %v", ErrSchemaMismatch, l.path, err)
	}
	return checkVersion(have)
}

// ID returns the ledger's identity: a UUID generated when the ledger was initialised.
//
// A rebuilt ledger gets a new one. The operational store pins the id it was derived
// from, so a tracking file left over from an earlier ledger is refused instead of
// being read as an index of this one.
func (l *Ledger) ID(ctx context.Context) (string, error) {
	var id string
	err := l.db.QueryRowContext(ctx,
		`SELECT value FROM ledger_meta WHERE key = ?`, metaLedgerID).Scan(&id)
	if err != nil {
		if isNoSuchTable(err) || errors.Is(err, sql.ErrNoRows) {
			return "", fmt.Errorf("%w: %s", ErrNotInitialised, l.path)
		}
		return "", classify("read ledger id", err)
	}
	return id, nil
}

// Drop removes every ledger table. It exists for the rebuild test (drop, init,
// re-ingest, compare the fold; MVP.md §7) and nothing in the CLI calls it. It drops
// tables, never rows: append-only governs the rows, and no method here removes one.
// The handle stays usable, and Init on it starts a new ledger with a new id.
func (l *Ledger) Drop(ctx context.Context) error {
	if _, err := l.db.ExecContext(ctx, dropSchema); err != nil {
		return classify("drop", err)
	}
	return nil
}

// Head reads the current watermark: the newest anam-event's seq, or 0 at genesis.
func (l *Ledger) Head(ctx context.Context) (Watermark, error) {
	return head(ctx, l.db)
}

// querier is what *sql.DB and *sql.Tx share, so a read can run inside or outside a
// transaction.
type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

func head(ctx context.Context, q querier) (Watermark, error) {
	var w Watermark
	err := q.QueryRowContext(ctx, `SELECT COALESCE(MAX(seq), 0) FROM anam_event`).Scan(&w)
	if err != nil {
		if isNoSuchTable(err) {
			return 0, ErrNotInitialised
		}
		return 0, classify("read watermark", err)
	}
	return w, nil
}

// Path reports the file this ledger lives in.
func (l *Ledger) Path() string { return l.path }

// Close releases the handle. When the last connection to the file closes, SQLite
// folds the write-ahead log back into it.
func (l *Ledger) Close() error {
	if err := l.db.Close(); err != nil {
		return fmt.Errorf("ledger: close: %w", err)
	}
	return nil
}

// classify wraps a database error with what was being attempted, and gives lock
// contention its own sentinel so an operator sees "another writer", not a bare code.
func classify(doing string, err error) error {
	if code, ok := primaryCode(err); ok && code == sqlite3.SQLITE_BUSY {
		return fmt.Errorf("%w: %s: %v", ErrBusy, doing, err)
	}
	return fmt.Errorf("ledger: %s: %w", doing, err)
}

// primaryCode extracts SQLite's primary result code. The driver reports extended
// codes, whose low byte is the primary one.
func primaryCode(err error) (int, bool) {
	var se *sqlite.Error
	if !errors.As(err, &se) {
		return 0, false
	}
	return se.Code() & 0xff, true
}

// isNoSuchTable reports whether err is SQLite saying a table does not exist. SQLite
// has no result code for it, only SQLITE_ERROR and this message.
func isNoSuchTable(err error) bool {
	code, ok := primaryCode(err)
	return ok && code == sqlite3.SQLITE_ERROR && strings.Contains(err.Error(), "no such table")
}
