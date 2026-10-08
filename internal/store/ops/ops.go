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
const schemaVersion = 6

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

// schema is executed idempotently by Open. Sort-ordered text columns (fact_time,
// anam_time) hold fixed-width clock.Layout timestamps; bucket ids are span.Bucket
// ids — both compare lexically in chronological order by construction.
const schema = `
-- tier: derived — the facet index: one row per logical facet, at its current version
-- and folded status (current_version + status is exactly a persisted fold.State,
-- which is what lets catch-up replay incrementally instead of from genesis).
-- The read paths that left the ledger live here: the timeline (fact_time), the type
-- filter, and bucket membership (the bucket-span seam — bucket_id is arithmetic on
-- fact_time, materialised for the joins the mint will need).
CREATE TABLE IF NOT EXISTS facet_index (
  facet_id        TEXT PRIMARY KEY,
  type            TEXT NOT NULL,
  fact_time      TEXT NOT NULL,
  bucket_id       TEXT NOT NULL,
  current_version INTEGER NOT NULL,
  status          TEXT NOT NULL,
  content_hash    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS facet_index_by_time   ON facet_index (fact_time);
CREATE INDEX IF NOT EXISTS facet_index_by_type   ON facet_index (type, fact_time);
CREATE INDEX IF NOT EXISTS facet_index_by_bucket ON facet_index (bucket_id);

-- tier: derived — the anam-event index: changes_since by anam-time, and the
-- as-of translation (greatest seq with anam_time <= T).
CREATE TABLE IF NOT EXISTS anam_event_index (
  seq              INTEGER PRIMARY KEY,
  anam_time      TEXT NOT NULL,
  kind             TEXT NOT NULL,
  facet_id         TEXT NOT NULL,
  artifact_version INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS anam_event_by_time ON anam_event_index (anam_time, seq);

-- tier: derived — the checkpoint index: every mint of every span, append-only.
-- covers_w is the watermark the value actually folded; value_hash addresses the
-- checkpoint file. As-of wants "greatest covers_w <= W", which is why this is a log
-- and the head is a view.
CREATE TABLE IF NOT EXISTS checkpoint (
  span_id    TEXT NOT NULL,
  v          INTEGER NOT NULL,
  covers_w   INTEGER NOT NULL,
  value_hash TEXT NOT NULL,
  minted_at  TEXT NOT NULL,
  PRIMARY KEY (span_id, v)
);

-- The head of each span is its latest checkpoint. A view, not a table: a second,
-- overwritten copy of the same fact could drift from the log it summarises.
CREATE VIEW IF NOT EXISTS span_head AS
  SELECT span_id, v, covers_w, value_hash, minted_at
  FROM checkpoint c
  WHERE v = (SELECT MAX(v) FROM checkpoint WHERE span_id = c.span_id);

-- tier: derived — one row per mint run that wrote. Every span's latest checkpoint at or
-- below a run's w was checked against the fold at w, minted or found unchanged, so a
-- read at or after w can start from those checkpoints and replay only what came after.
-- A run is recorded after its checkpoints, so a run still in progress is invisible.
-- Added without a schema version: the table is additive, and a store without it simply
-- has no runs, which reads as "no checkpoint to start from".
CREATE TABLE IF NOT EXISTS mint_run (
  w      INTEGER PRIMARY KEY,
  ran_at TEXT NOT NULL
);

-- tier: derived — how many checkpoint rows the log held when a run at w was recorded.
-- A run's own rows and an earlier run's share a covers_w when both mint at one W (a
-- lowered budget splits with no new event), so covers_w cannot say a row was left by a
-- run that crashed. The count can: a log holding more rows than its newest seal has
-- checkpoints no recorded run accounts for, and nothing starts from it until a full
-- mint seals it again. Additive, like mint_run: a store without seals mints in full.
CREATE TABLE IF NOT EXISTS mint_seal (
  checkpoints INTEGER PRIMARY KEY,
  w           INTEGER NOT NULL
);

-- tier: derived — how many facets each checkpoint's value holds, so a read can count
-- the whole value without decoding the buckets it does not show. Additive, like
-- mint_run: a checkpoint without a row here is counted by decoding it.
CREATE TABLE IF NOT EXISTS checkpoint_size (
  span_id TEXT NOT NULL,
  v       INTEGER NOT NULL,
  facets  INTEGER NOT NULL,
  PRIMARY KEY (span_id, v)
);

-- tier: derived — per-source resumption markers. Losing one costs a re-scan, never a
-- fact: ingest is idempotent.
CREATE TABLE IF NOT EXISTS sync_cursor (
  source     TEXT PRIMARY KEY,
  cursor     TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

-- tier: derived — how far the derived tier has replayed the ledger. Catch-up asserts
-- last_seq == W before any fold; behind is fixable, wrong is not representable.
CREATE TABLE IF NOT EXISTS replay_cursor (
  id       INTEGER PRIMARY KEY CHECK (id = 1),
  last_seq INTEGER NOT NULL
);

-- tier: provisioned — Facet Definitions, every version, loaded from embedded JSON.
-- Not in the ledger: a definition is not something Anam knew, it is how Anam
-- interprets. Old artifacts stay interpretable because old versions stay loadable.
CREATE TABLE IF NOT EXISTS definition (
  type    TEXT NOT NULL,
  version INTEGER NOT NULL,
  body    TEXT NOT NULL,
  PRIMARY KEY (type, version)
);

-- tier: provisioned — the static source -> facet-type token map (the gate's
-- authenticated pillar). One row per grant.
CREATE TABLE IF NOT EXISTS token_scope (
  source TEXT NOT NULL,
  type   TEXT NOT NULL,
  PRIMARY KEY (source, type)
);

-- tier: authored — Anam's own bookkeeping; comes back from a backup or not at all.
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

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

	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("ops: open %q: %w", path, err)
	}

	if _, err := db.ExecContext(ctx, schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ops: apply schema: %w", err)
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

// pinSchemaVersion records the version on first open and refuses a mismatch after.
func pinSchemaVersion(ctx context.Context, db *sql.DB) error {
	stored, fresh, err := pin(ctx, db, "schema_version", fmt.Sprintf("%d", schemaVersion))
	switch {
	case err != nil:
		return fmt.Errorf("ops: pin schema version: %w", err)
	case fresh || stored == fmt.Sprintf("%d", schemaVersion):
		return nil
	}
	return fmt.Errorf("%w: database holds %s, this build writes %d — delete the file and re-run init; the derived tier replays and the provisioned tier reloads",
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

// DefinitionRow is one stored Definition: the manifest as loaded, by type and
// version. The store keeps every version it has seen so that a later build can be
// checked against the interpretation the ledger's facets were keyed under.
type DefinitionRow struct {
	Type    string
	Version int
	Body    string
}

// Definitions returns every stored Definition, in type then version order.
func (d *DB) Definitions(ctx context.Context) ([]DefinitionRow, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT type, version, body FROM definition ORDER BY type, version`)
	if err != nil {
		return nil, fmt.Errorf("ops: read definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DefinitionRow
	for rows.Next() {
		var r DefinitionRow
		if err := rows.Scan(&r.Type, &r.Version, &r.Body); err != nil {
			return nil, fmt.Errorf("ops: scan definition: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: read definitions: %w", err)
	}
	return out, nil
}

// PutDefinitions stores the given Definitions, replacing a stored body at the same
// type and version. The provisioned tier is a copy of what the binary carries, so
// overwriting it is a reload, not a loss; the guard that decides whether the reload
// is safe runs before this is called.
func (d *DB) PutDefinitions(ctx context.Context, defs []DefinitionRow) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ops: put definitions: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, r := range defs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO definition (type, version, body) VALUES (?, ?, ?)
			 ON CONFLICT(type, version) DO UPDATE SET body = excluded.body`,
			r.Type, r.Version, r.Body); err != nil {
			return fmt.Errorf("ops: put definition %s v%d: %w", r.Type, r.Version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ops: put definitions: %w", err)
	}
	return nil
}

// Close releases the handle.
func (d *DB) Close() error { return d.db.Close() }

// SQL exposes the underlying handle to sibling store packages and tests. The domain
// never sees it: only internal/store may speak SQL, and the boundary check holds
// everything above to that.
func (d *DB) SQL() *sql.DB { return d.db }
