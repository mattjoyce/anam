package ledger

// schemaVersion is the version this build writes. A fresh ledger pins it in
// ledger_meta; an older ledger reaches it by migration (migrate.go), and every open
// compares the file's effective version against it.
//
// Bump it whenever a stored object is renamed or reshaped, and add the step to
// migrations. Every object below is created IF NOT EXISTS, so without the pin a
// renamed table would be created empty beside the populated one and the open would
// report success.
//
// History:
//
//	1  the three tables and their triggers
//	2  facet.series, the Definition's series stamp (SELF-LOG.md §2)
//	3  no DDL: the identity encoding (facet.SourceRef, length-prefixed values) and
//	   the fact-time zone changed what a facet's id and fact_time are. A schema 2
//	   file holds rows keyed by the old rules and cannot be carried forward; it is
//	   refused with the remedy, a new ledger from the same sources.
const schemaVersion = 3

// The ledger's schema. Three tables, and every one of them is append-only.
//
// Keys are typed. artifact_version and seq are INTEGER columns, so they order
// numerically and need no zero padding. Timestamps stay fixed-width text
// (clock.Layout), because text timestamps are compared lexically wherever they are
// read.
//
// The watermark W has no row of its own. It is MAX(seq), which the seq trigger keeps
// gapless, so W and the newest event's seq are one number held in one place.
//
// What the triggers hold, for every writer including a sqlite3 shell:
//
//   - no row is updated or deleted
//   - INSERT OR REPLACE cannot overwrite a row. REPLACE removes the old row without
//     firing the delete trigger unless recursive_triggers is on, and that pragma is
//     per connection, so the insert trigger refuses an existing key itself
//   - an anam-event's seq is exactly one more than the newest seq
//   - an artifact at version n > 1 has version n-1 beneath it, which is what lets
//     KnownFacets treat a missing v1 as "never admitted"
//
// A trigger is not a security boundary. Whoever holds the file can drop it. The threat
// is a refactor or a hurried shell session, and against that the database refusing the
// write is stronger than a test that has to be run.
const schema = `
CREATE TABLE IF NOT EXISTS ledger_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS facet (
  facet_id           TEXT    NOT NULL,
  artifact_version   INTEGER NOT NULL CHECK (artifact_version >= 1),
  type               TEXT    NOT NULL,
  confidence         REAL    NOT NULL,
  classification     TEXT    NOT NULL,
  definition_version INTEGER NOT NULL,
  fact_time          TEXT    NOT NULL,
  series             TEXT    NOT NULL DEFAULT '',
  binding_person_id  TEXT    NOT NULL,
  binding_assurance  TEXT    NOT NULL,
  binding_span_hint  TEXT    NOT NULL DEFAULT '',
  prov_source        TEXT    NOT NULL,
  prov_injector      TEXT    NOT NULL,
  prov_source_ref    TEXT    NOT NULL,
  prov_method        TEXT    NOT NULL DEFAULT '',
  prov_model         TEXT    NOT NULL DEFAULT '',
  prov_watermark     TEXT    NOT NULL DEFAULT '',
  source_schema      TEXT    NOT NULL,
  schema_version     INTEGER NOT NULL,
  content_hash       TEXT    NOT NULL,
  payload_kind       TEXT    NOT NULL,
  media_type         TEXT    NOT NULL,
  payload            BLOB    NOT NULL,
  PRIMARY KEY (facet_id, artifact_version)
);

CREATE TABLE IF NOT EXISTS anam_event (
  seq                    INTEGER PRIMARY KEY CHECK (seq >= 1),
  kind                   TEXT    NOT NULL,
  facet_id               TEXT    NOT NULL,
  anam_time              TEXT    NOT NULL,
  artifact_version       INTEGER NOT NULL,
  prior_artifact_version INTEGER NOT NULL DEFAULT 0
);

CREATE TRIGGER IF NOT EXISTS ledger_meta_no_update BEFORE UPDATE ON ledger_meta
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: ledger_meta rows are never updated'); END;
CREATE TRIGGER IF NOT EXISTS ledger_meta_no_delete BEFORE DELETE ON ledger_meta
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: ledger_meta rows are never deleted'); END;
CREATE TRIGGER IF NOT EXISTS ledger_meta_no_replace BEFORE INSERT ON ledger_meta
WHEN EXISTS (SELECT 1 FROM ledger_meta WHERE key = NEW.key)
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: ledger_meta key already pinned'); END;

CREATE TRIGGER IF NOT EXISTS facet_no_update BEFORE UPDATE ON facet
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: facet rows are never updated'); END;
CREATE TRIGGER IF NOT EXISTS facet_no_delete BEFORE DELETE ON facet
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: facet rows are never deleted'); END;
CREATE TRIGGER IF NOT EXISTS facet_no_replace BEFORE INSERT ON facet
WHEN EXISTS (SELECT 1 FROM facet
             WHERE facet_id = NEW.facet_id AND artifact_version = NEW.artifact_version)
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: artifact version already written'); END;
CREATE TRIGGER IF NOT EXISTS facet_version_follows BEFORE INSERT ON facet
WHEN NEW.artifact_version > 1
 AND NOT EXISTS (SELECT 1 FROM facet
                 WHERE facet_id = NEW.facet_id AND artifact_version = NEW.artifact_version - 1)
BEGIN SELECT RAISE(ABORT, 'ledger: artifact version does not follow the prior version'); END;

CREATE TRIGGER IF NOT EXISTS anam_event_no_update BEFORE UPDATE ON anam_event
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: anam_event rows are never updated'); END;
CREATE TRIGGER IF NOT EXISTS anam_event_no_delete BEFORE DELETE ON anam_event
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: anam_event rows are never deleted'); END;
CREATE TRIGGER IF NOT EXISTS anam_event_seq_follows BEFORE INSERT ON anam_event
WHEN NEW.seq IS NOT (SELECT COALESCE(MAX(seq), 0) + 1 FROM anam_event)
BEGIN SELECT RAISE(ABORT, 'ledger: anam-event seq does not follow the newest seq'); END;
`

// indexes are the ledger's lookups. They hold no rows and no fact, every read is
// correct without them, and they fire no trigger, so they are not schema in the sense
// schemaVersion pins: adding one changes how fast the ledger answers, never what. Every
// writer creates them when it opens the file (ensureIndexes), so a ledger written by an
// older build gains them on its next writable open without a migration, and a
// read-only handle on a file that lacks one is slower, not wrong.
//
//	anam_event_by_facet  one facet's events in seq order, for the gate's retraction
//	                     check, which folds that facet and nothing else (card 65)
const indexes = `
CREATE INDEX IF NOT EXISTS anam_event_by_facet ON anam_event (facet_id, seq);
`

// dropSchema removes every ledger object. DROP TABLE takes a table's triggers with it
// and fires none of them, which is why Drop can remove what no statement can delete
// from.
const dropSchema = `
DROP TABLE IF EXISTS facet;
DROP TABLE IF EXISTS anam_event;
DROP TABLE IF EXISTS ledger_meta;
`

// The ledger_meta keys. schema_version is the version the file was created at and
// never changes, because ledger_meta is write-once; each migration applied since adds
// a schema_migration:<n> key, and the effective version is the highest of them all.
const (
	metaSchemaVersion   = "schema_version"
	metaPersonID        = "person_id"
	metaLedgerID        = "ledger_id"
	metaMigrationPrefix = "schema_migration:"
)
