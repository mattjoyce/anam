package ledger

import _ "embed" // go:embed for the SQL files beside this one

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
//
// It names the file's layout, so it is never configurable (card 151).
const schemaVersion = 3

// The ledger's schema, held whole in ledger.sql beside this file and embedded so the
// binary that checks schemaVersion carries the text it checks. Three tables, and every
// one of them is append-only.
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
//
//go:embed ledger.sql
var schema string

// indexes are the ledger's lookups, held in indexes.sql and kept apart from ledger.sql
// because they are installed by a separate step. They hold no rows and no fact, every
// read is correct without them, and they fire no trigger, so they are not schema in the sense
// schemaVersion pins: adding one changes how fast the ledger answers, never what. Every
// writer creates them when it opens the file (ensureIndexes), so a ledger written by an
// older build gains them on its next writable open without a migration, and a
// read-only handle on a file that lacks one is slower, not wrong.
//
//	anam_event_by_facet  one facet's events in seq order, for the gate's retraction
//	                     check, which folds that facet and nothing else (card 65)
//
//go:embed indexes.sql
var indexes string

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
