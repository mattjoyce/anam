
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

-- tier: derived — the edge index (card 104): one row per end of every live edge facet,
-- the facet ids its payload names on its from and to sides, so "the live edges touching
-- X" is one lookup. Derived from the edges' own payloads by catch-up, replaced whole
-- for an edge at each of its versions, gone when it is tombstoned.
CREATE TABLE IF NOT EXISTS facet_edge (
  edge_id  TEXT NOT NULL,
  kind     TEXT NOT NULL,
  side     TEXT NOT NULL CHECK (side IN ('from', 'to')),
  facet_id TEXT NOT NULL,
  PRIMARY KEY (edge_id, side, facet_id)
);
CREATE INDEX IF NOT EXISTS facet_edge_by_facet ON facet_edge (facet_id);

-- tier: derived, the checkpoint index: every mint of every span. A cache that may go
-- missing: a mint removes the rows the newest run does not read (CollectCheckpoints),
-- and a read that needs one that is gone folds the ledger in full.
-- covers_w is the watermark the value actually folded; value_hash addresses the
-- checkpoint's value in checkpoint_value. As-of wants "greatest covers_w <= W", which
-- is why this is a log and the head is a view.
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

-- tier: derived, checkpoint values (card 215): the bytes a checkpoint row names, keyed
-- by the SHA-256 of themselves. Insert-only: an update is refused. A value is
-- re-hashed on every read, so a row that no longer hashes to its key is reported
-- rather than served. Being derived it comes back by minting, and a store rebuild may
-- drop it.
CREATE TABLE IF NOT EXISTS checkpoint_value (
  hash TEXT PRIMARY KEY CHECK (length(hash) = 71 AND hash GLOB 'sha256:*'),
  body BLOB NOT NULL
);
CREATE TRIGGER IF NOT EXISTS checkpoint_value_no_update
BEFORE UPDATE ON checkpoint_value
BEGIN SELECT RAISE(ABORT, 'checkpoint_value is insert-only'); END;

-- tier: derived, one row per mint run that wrote. Every span's latest checkpoint at or
-- below a run's w was checked against the fold at w, minted or found unchanged, so a
-- read at or after w can start from those checkpoints and replay only what came after.
-- A run is recorded after its checkpoints, so a run still in progress is invisible.
-- picks is a digest of the checkpoints the run took (span, version and value hash),
-- written with the run. A read that starts from a run takes the same checkpoints again
-- and compares: a row lost, replaced or added since changes the digest, and the read
-- declines and folds the ledger in full instead of composing from a log that is not
-- the one the run saw (RunCheckpoints). This is also how a mint that crashed before it
-- recorded its run is told from one that finished (LogSealed).
CREATE TABLE IF NOT EXISTS mint_run (
  w      INTEGER PRIMARY KEY,
  ran_at TEXT NOT NULL,
  picks  TEXT NOT NULL
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

-- tier: provisioned — Facet Definitions, every version, loaded from embedded JSON:
-- each type's current manifest and the earlier versions the binary keeps under
-- manifests/history (card 111), so a rebuilt store holds every version a facet was
-- admitted under. Not in the ledger: a definition is not something Anam knew, it is
-- how Anam interprets. Old artifacts stay interpretable because old versions stay
-- loadable.
CREATE TABLE IF NOT EXISTS definition (
  type    TEXT NOT NULL,
  version INTEGER NOT NULL,
  body    TEXT NOT NULL,
  PRIMARY KEY (type, version)
);

-- tier: provisioned — projection templates, every version, as the catalog carried them
-- when the daemon last started: current ones and the history beside them. Stored by
-- upsert and never swept, as definition is, so a template whose file has left the
-- catalog still lets a receipt that names it verify. A file written before this table
-- gains it empty and is filled at the next start; every body it would have held was in
-- that build's own catalog, so absent here never stands for a body that is gone.
CREATE TABLE IF NOT EXISTS projection (
  name    TEXT NOT NULL,
  version INTEGER NOT NULL,
  body    TEXT NOT NULL,
  PRIMARY KEY (name, version)
);

-- tier: provisioned — Facet Definitions registered by importers (POST /v1/definitions),
-- every version. Each importer's repo holds its Definitions, so a lost table comes
-- back by registering them again. A registered version is immutable: the triggers
-- refuse a change or a removal, for every writer.
CREATE TABLE IF NOT EXISTS registered_definition (
  type          TEXT NOT NULL,
  version       INTEGER NOT NULL,
  body          TEXT NOT NULL,
  registered_at TEXT NOT NULL,
  PRIMARY KEY (type, version)
);
CREATE TRIGGER IF NOT EXISTS registered_definition_no_update
BEFORE UPDATE ON registered_definition
BEGIN SELECT RAISE(ABORT, 'registered_definition is immutable'); END;
CREATE TRIGGER IF NOT EXISTS registered_definition_no_delete
BEFORE DELETE ON registered_definition
BEGIN SELECT RAISE(ABORT, 'registered_definition is immutable'); END;

-- tier: authored — projection templates the operator registered (POST /v1/projections),
-- every version (card 125). The bundled templates are the floor and come back with the
-- binary; a registered one comes back from a backup, or by registering it again from
-- wherever the operator keeps the file. A registered version is immutable: the
-- triggers refuse a change or a removal, for every writer.
CREATE TABLE IF NOT EXISTS registered_projection (
  name          TEXT NOT NULL,
  version       INTEGER NOT NULL,
  body          TEXT NOT NULL,
  registered_at TEXT NOT NULL,
  PRIMARY KEY (name, version)
);
CREATE TRIGGER IF NOT EXISTS registered_projection_no_update
BEFORE UPDATE ON registered_projection
BEGIN SELECT RAISE(ABORT, 'registered_projection is immutable'); END;
CREATE TRIGGER IF NOT EXISTS registered_projection_no_delete
BEFORE DELETE ON registered_projection
BEGIN SELECT RAISE(ABORT, 'registered_projection is immutable'); END;

-- tier: authored, receipts (card 215): the immutable record of one serve, keyed by the
-- SHA-256 of its own bytes. What Anam showed is not something the ledger records, so a
-- receipt comes back from a backup or from nowhere. Immutable: the triggers refuse a
-- change or a removal, for every writer.
CREATE TABLE IF NOT EXISTS receipt (
  hash TEXT PRIMARY KEY CHECK (length(hash) = 71 AND hash GLOB 'sha256:*'),
  body BLOB NOT NULL
);
CREATE TRIGGER IF NOT EXISTS receipt_no_update
BEFORE UPDATE ON receipt
BEGIN SELECT RAISE(ABORT, 'receipt is immutable'); END;
CREATE TRIGGER IF NOT EXISTS receipt_no_delete
BEFORE DELETE ON receipt
BEGIN SELECT RAISE(ABORT, 'receipt is immutable'); END;

-- tier: authored, served values (card 215): the bytes a receipt's served_hash names,
-- keyed by the SHA-256 of themselves, so a receipt can be shown without the ledger.
-- Immutable, like the receipt.
CREATE TABLE IF NOT EXISTS served_value (
  hash TEXT PRIMARY KEY CHECK (length(hash) = 71 AND hash GLOB 'sha256:*'),
  body BLOB NOT NULL
);
CREATE TRIGGER IF NOT EXISTS served_value_no_update
BEFORE UPDATE ON served_value
BEGIN SELECT RAISE(ABORT, 'served_value is immutable'); END;
CREATE TRIGGER IF NOT EXISTS served_value_no_delete
BEFORE DELETE ON served_value
BEGIN SELECT RAISE(ABORT, 'served_value is immutable'); END;

-- tier: authored, saved selections (card 215): a program's canonical text, keyed by the
-- SHA-256 of it. Immutable; a selection is named by a code, and a code never changes
-- meaning.
CREATE TABLE IF NOT EXISTS saved_selection (
  hash TEXT PRIMARY KEY CHECK (length(hash) = 71 AND hash GLOB 'sha256:*'),
  body BLOB NOT NULL
);
CREATE TRIGGER IF NOT EXISTS saved_selection_no_update
BEFORE UPDATE ON saved_selection
BEGIN SELECT RAISE(ABORT, 'saved_selection is immutable'); END;
CREATE TRIGGER IF NOT EXISTS saved_selection_no_delete
BEFORE DELETE ON saved_selection
BEGIN SELECT RAISE(ABORT, 'saved_selection is immutable'); END;

-- tier: authored, the codes of saved selections that lengthened past their first
-- digits (card 215), each a small JSON record keyed by the SHA-256 of it. Which of two
-- selections that share a code's start was saved first is a fact the hashes cannot
-- say, so the record is kept. Immutable.
CREATE TABLE IF NOT EXISTS saved_selection_code (
  hash TEXT PRIMARY KEY CHECK (length(hash) = 71 AND hash GLOB 'sha256:*'),
  body BLOB NOT NULL
);
CREATE TRIGGER IF NOT EXISTS saved_selection_code_no_update
BEFORE UPDATE ON saved_selection_code
BEGIN SELECT RAISE(ABORT, 'saved_selection_code is immutable'); END;
CREATE TRIGGER IF NOT EXISTS saved_selection_code_no_delete
BEFORE DELETE ON saved_selection_code
BEGIN SELECT RAISE(ABORT, 'saved_selection_code is immutable'); END;

-- tier: authored, inline projections (card 211): the body of a projection a query carried
-- in its request rather than naming a registered one, keyed by the SHA-256 of it, which is
-- the template hash its receipt records. A receipt for such a query names this row, so
-- verify finds the template to replay. Immutable, like the receipt; a body is only ever
-- filed when a query that used it was answered.
CREATE TABLE IF NOT EXISTS inline_projection (
  hash TEXT PRIMARY KEY CHECK (length(hash) = 71 AND hash GLOB 'sha256:*'),
  body BLOB NOT NULL
);
CREATE TRIGGER IF NOT EXISTS inline_projection_no_update
BEFORE UPDATE ON inline_projection
BEGIN SELECT RAISE(ABORT, 'inline_projection is immutable'); END;
CREATE TRIGGER IF NOT EXISTS inline_projection_no_delete
BEFORE DELETE ON inline_projection
BEGIN SELECT RAISE(ABORT, 'inline_projection is immutable'); END;

-- tier: provisioned — the static source -> facet-type token map (the gate's
-- authenticated pillar). One row per grant.
CREATE TABLE IF NOT EXISTS token_scope (
  source TEXT NOT NULL,
  type   TEXT NOT NULL,
  PRIMARY KEY (source, type)
);

-- tier: disposable — importers' complete-resend snapshots (card 78): which parts of a
-- run were admitted and which facet ids they carried. Lost, the next resend makes
-- another; nothing is ever tombstoned from here without an operator running
-- anam retract --yes. An incomplete snapshot is dropped after a day.
CREATE TABLE IF NOT EXISTS snapshot (
  id           TEXT PRIMARY KEY,
  source       TEXT NOT NULL,
  types        TEXT NOT NULL,
  parts        INTEGER NOT NULL CHECK (parts >= 1),
  started_at   TEXT NOT NULL,
  started_w    INTEGER NOT NULL,
  completed_at TEXT,
  completed_w  INTEGER
);
CREATE INDEX IF NOT EXISTS snapshot_by_source ON snapshot (source, completed_at);
-- A part is a value: digest is the content identity of the envelopes it was recorded
-- with (card 107), so a resend of the part number with other envelopes is refused and
-- a snapshot's membership, the union of its parts, cannot change once a part is in.
-- Added in place by bindSnapshotParts rather than by a schema version: the tier is
-- disposable, and a version bump rebuilds the provisioned tier too (card 111).
CREATE TABLE IF NOT EXISTS snapshot_part (
  id     TEXT NOT NULL,
  part   INTEGER NOT NULL,
  digest TEXT NOT NULL,
  PRIMARY KEY (id, part)
);
CREATE TABLE IF NOT EXISTS snapshot_facet (
  id       TEXT NOT NULL,
  facet_id TEXT NOT NULL,
  PRIMARY KEY (id, facet_id)
);

-- tier: authored — Anam's own bookkeeping; comes back from a backup or not at all.
CREATE TABLE IF NOT EXISTS meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

-- tier: authored — operator admission holds (POST /v1/hold). Holds are per-source
-- or global ("*"). Held rows are not updated; deletion releases a hold.
CREATE TABLE IF NOT EXISTS admission_hold (
  source    TEXT PRIMARY KEY,
  held_at   TEXT NOT NULL
);
CREATE TRIGGER IF NOT EXISTS admission_hold_no_update
BEFORE UPDATE ON admission_hold
BEGIN SELECT RAISE(ABORT, 'admission_hold is immutable'); END;
