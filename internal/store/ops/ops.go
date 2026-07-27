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
const schemaVersion = 5

// ErrSchemaVersion is returned by Open for a database written by a different schema.
var ErrSchemaVersion = errors.New("ops: schema version mismatch")

// ErrBucketWidth is returned by Open for an index built at a different bucket width.
var ErrBucketWidth = errors.New("ops: bucket width mismatch")

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
