// Package ops is the SQLite half of Anam's store: the operational database
// (ARCHITECTURE.md §8).
//
// It is a real store, not a cache. Each table is annotated with its tier — where its
// contents come back from — because that is the only thing that tells a later reader
// whether dropping it is survivable:
//
//   - derived     — rebuilt by replaying the ledger. The rebuild test drops this tier.
//   - provisioned — reloaded from embedded assets and config; i.e. from git.
//   - authored    — comes back from a backup, or from nowhere.
//
// The derived tier must stay provably derived: if a facet payload, an anam-event or
// the watermark ever acquired a second home here, the rebuild test would pass on data
// it never rebuilt.
package ops

import (
	"context"
	"database/sql"
	_ "embed" // go:embed for ops.sql
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // registers the pure-Go "sqlite" driver

	"github.com/mattjoyce/anam/internal/span"
)

// schemaVersion is bumped on any schema change. The prototype recreates rather than
// migrates: everything derived replays, everything provisioned reloads.
//
// 4 -> 5 adds no table. It bumps because meta now carries a pinned bucket_width, and a
// database written before that pin has none — so opening one would record whatever
// width the config happened to say and accept an index built at another. The bump
// makes the pre-pin database refuse, which is the same answer by a route that works.
//
// 5 -> 6 is the same move for the ledger pin. A database written before it names no
// ledger, so the first catch-up would pin whichever ledger it met and read an index
// derived from another as an index of that one.
//
// registered_definition (card 53) and the snapshot tables (card 78) do not bump it. Open creates the table and its
// triggers in a version-6 file as in a new one, and a build that predates it never
// reads it, so neither build can misread the other's file. It names the file's
// layout, so it is never configurable (card 151).
//
// 7 -> 8 adds facet_edge (card 104). A table added without a bump would be empty in a
// file that had already replayed past the edges it should hold, and for edges absent
// does not mean none, so the pre-edge file refuses and is replayed.
//
// 8 -> 9 adds checkpoint_value, receipt, served_value, saved_selection and
// saved_selection_code (card 215): the values, receipts and selections that were files
// under an artifact directory. The same rule bumps it. A version-8 file has checkpoint
// rows naming values it does not hold, and receipts and selections that exist only in
// the folder beside it, so a build that opened it would find them absent and answer
// as though there were none. The file refuses, and is carried forward by rebuilding
// it and moving the folder in (anam move-artifacts). The bump also gives mint_run its
// picks column and drops mint_seal, which the picks replace.
const schemaVersion = 9

// ErrSchemaVersion is returned by Open for a database written by a different schema.
var ErrSchemaVersion = errors.New("ops: schema version mismatch")

// ErrBucketWidth is returned by Open for an index built at a different bucket width.
var ErrBucketWidth = errors.New("ops: bucket width mismatch")

// ErrLedgerChanged is returned by PinLedger when the derived tier was built from a
// different ledger than the one it is being asked to follow.
var ErrLedgerChanged = errors.New("ops: derived tier belongs to a different ledger")

// DB is a handle on the operational store.
type DB struct {
	db *sql.DB
}

// schema is ops.sql, embedded, and executed idempotently by Open. Sort-ordered text
// columns (fact_time, anam_time) hold fixed-width clock.Layout timestamps; bucket ids
// are span.Bucket ids — both compare lexically in chronological order by construction.
//
//go:embed ops.sql
var schema string

// Open opens (creating if absent) the operational store at path and idempotently
// applies the schema. Parent directories are created. A database written by a
// different schema version, or built at a different bucket width, is refused rather
// than migrated silently.
//
// buckets is pinned rather than merely used because facet_index.bucket_id is
// persisted, and catch-up only rewrites the rows new events touch. Opening the same
// database at another width would therefore leave old ids and new ids side by side,
// each well-formed, with nothing to say which is which.
func Open(ctx context.Context, path string, buckets span.Buckets) (*DB, error) {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, fmt.Errorf("ops: create data dir: %w", err)
		}
	}

	// busy_timeout is not a config key (card 151): the caller's context, from
	// timeout, bounds every call, and the daemon is the store's one writer.
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("ops: open %q: %w", path, err)
	}

	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ops: apply schema: %w", err)
	}
	if err := bindSnapshotParts(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := pinSchemaVersion(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	if err := pinBucketWidth(ctx, db, buckets.Width()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &DB{db: db}, nil
}

// OpenReadOnly opens an operational store that Open has already prepared, on a handle
// SQLite will not accept a write from. It is for a process that also holds Open's
// handle and wants its reads to be reads by construction, not by convention.
func OpenReadOnly(ctx context.Context, path string) (*DB, error) {
	escaped := strings.NewReplacer("%", "%25", "?", "%3f", "#", "%23").Replace(path)
	db, err := sql.Open("sqlite", "file:"+escaped+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("ops: open %q read-only: %w", path, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ops: open %q read-only: %w", path, err)
	}
	return &DB{db: db}, nil
}

// bindSnapshotParts gives a store written before card 107 the digest column on
// snapshot_part. A part recorded without one cannot be told from a changed one, so
// the snapshot tables are cleared with it: the tier is disposable, and the next
// resend remakes them. A store that has the column is left alone.
func bindSnapshotParts(ctx context.Context, db *sql.DB) error {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('snapshot_part') WHERE name = 'digest'`).Scan(&n); err != nil {
		return fmt.Errorf("ops: snapshot_part columns: %w", err)
	}
	if n > 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ops: bind snapshot parts: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, q := range []string{
		`DELETE FROM snapshot_facet`, `DELETE FROM snapshot_part`, `DELETE FROM snapshot`,
		`ALTER TABLE snapshot_part ADD COLUMN digest TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("ops: bind snapshot parts: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ops: bind snapshot parts: %w", err)
	}
	return nil
}

// pinSchemaVersion records the version on first open and refuses a mismatch after.
func pinSchemaVersion(ctx context.Context, db *sql.DB) error {
	stored, fresh, err := pin(ctx, db, "schema_version", fmt.Sprintf("%d", schemaVersion))
	switch {
	case err != nil:
		return fmt.Errorf("ops: pin schema version: %w", err)
	case fresh || stored == fmt.Sprintf("%d", schemaVersion):
		return nil
	}
	return fmt.Errorf("%w: database holds %s, this build writes %d. Set the file aside and re-run init; the derived tier replays and the provisioned tier reloads, "+
		"and anam move-artifacts --registered-from <the old file> copies across what importers and the operator registered, and moves in any artifact folder",
		ErrSchemaVersion, stored, schemaVersion)
}

// pinBucketWidth records the width the index was built at and refuses a mismatch
// after. Same remedy as a schema change, and for the same reason: the derived tier is
// rebuildable, so re-deriving it is cheaper than reasoning about a half-converted one.
func pinBucketWidth(ctx context.Context, db *sql.DB, w span.Width) error {
	stored, fresh, err := pin(ctx, db, "bucket_width", string(w))
	switch {
	case err != nil:
		return fmt.Errorf("ops: pin bucket width: %w", err)
	case fresh || stored == string(w):
		return nil
	}
	return fmt.Errorf("%w: index was built %s, config says %s — delete the file and re-run init; the derived tier replays at the new width",
		ErrBucketWidth, stored, w)
}

// PinLedger records which ledger the derived tier is derived from, on first use, and
// refuses any other afterwards.
//
// The replay cursor is only a number. Against a rebuilt or replaced ledger it can
// compare as "behind" or even "current" while every row beside it describes events
// the new ledger never held. The ledger's id is what tells the two apart. The remedy
// is the derived tier's usual one: it rebuilds by replay.
func (d *DB) PinLedger(ctx context.Context, ledgerID string) error {
	if ledgerID == "" {
		return fmt.Errorf("ops: pin ledger: empty ledger id")
	}
	stored, fresh, err := pin(ctx, d.db, "ledger_id", ledgerID)
	switch {
	case err != nil:
		return fmt.Errorf("ops: pin ledger: %w", err)
	case fresh || stored == ledgerID:
		return nil
	}
	return fmt.Errorf("%w: index was derived from ledger %s, this is ledger %s. Delete the tracking file and re-run init; the derived tier replays from this ledger",
		ErrLedgerChanged, stored, ledgerID)
}

// pin writes key on first open and reports what is stored thereafter. fresh is true
// when this call is the one that wrote it.
func pin(ctx context.Context, db *sql.DB, key, value string) (stored string, fresh bool, err error) {
	res, err := db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES (?, ?) ON CONFLICT (key) DO NOTHING`, key, value)
	if err != nil {
		return "", false, err
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return value, true, nil
	}
	if err := db.QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key = ?`, key).Scan(&stored); err != nil {
		return "", false, err
	}
	return stored, false, nil
}

// Close releases the handle.
func (d *DB) Close() error { return d.db.Close() }

// SQL exposes the underlying handle to sibling store packages and tests. The domain
// never sees it: only internal/store may speak SQL, and the boundary check holds
// everything above to that.
func (d *DB) SQL() *sql.DB { return d.db }
