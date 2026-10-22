
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
