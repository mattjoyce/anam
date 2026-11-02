PRAGMA foreign_keys=OFF;
BEGIN TRANSACTION;
CREATE TABLE ledger_meta (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
INSERT INTO ledger_meta VALUES('schema_version','3');
INSERT INTO ledger_meta VALUES('person_id','matt');
INSERT INTO ledger_meta VALUES('ledger_id','fa824c51-6b73-437b-a987-25396ea2ced0');
CREATE TABLE facet (
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
INSERT INTO facet VALUES('ed4af0f9-ae12-5def-8b77-1dfdfcb9f54e',1,'health.weight',1.0,'Sensitive',7,'2026-10-08T20:30:00.000000000Z','','matt','medium','','test-import','test-import','test-import:3:w-1','','','','records/health.weight',1,'sha256:89096746570e69759302c4095d301094ae451c97b7897be34b1718336acaa565','verbatim','application/json',X'7b2264617465223a22323032362d31302d30392030373a3330222c226964223a22772d31222c226b67223a38302e352c22736f757263655f74696d65223a7b226261736973223a22736f75726365222c226f6666736574223a222b31313a3030222c2276616c7565223a22323032362d31302d30392030373a3330222c227a6f6e65223a224175737472616c69612f5379646e6579227d7d');
INSERT INTO facet VALUES('15fd02c5-b872-5c78-853d-2128968b8bfa',1,'health.weight',1.0,'Sensitive',7,'2026-10-08T20:30:00.000000000Z','','matt','medium','','test-import','test-import','test-import:3:w-2','','','','records/health.weight',1,'sha256:d14ca628c146594b9955a693c944a67177cd560bcffb1ea31843bf6fe7a9ed92','verbatim','application/json',X'7b2264617465223a22323032362d31302d30392030373a3330222c226964223a22772d32222c226b67223a37392c22736f757263655f74696d65223a7b226261736973223a22736f75726365222c226f6666736574223a222b31313a3030222c2276616c7565223a22323032362d31302d30392030373a3330222c227a6f6e65223a224175737472616c69612f5379646e6579227d7d');
INSERT INTO facet VALUES('fe61ec2a-6d8b-56b0-8930-e3270f4ad44c',1,'health.weight',1.0,'Sensitive',7,'2026-10-08T20:30:00.000000000Z','','matt','medium','','test-import','test-import','test-import:3:w-3','','','','records/health.weight',1,'sha256:82375c588bd2c2af36cb21aa65d627bc9a4fa8b0457692787855043c01b76b44','verbatim','application/json',X'7b2264617465223a22323032362d31302d30392030373a3330222c226964223a22772d33222c226b67223a38312e322c22736f757263655f74696d65223a7b226261736973223a22736f75726365222c226f6666736574223a222b31313a3030222c2276616c7565223a22323032362d31302d30392030373a3330222c227a6f6e65223a224175737472616c69612f5379646e6579227d7d');
INSERT INTO facet VALUES('2f107449-3dc6-594e-9e52-916eb6008cf5',1,'test.reading',1.0,'Normal',1,'2026-10-01T07:00:00.000000000Z','','matt','medium','','test-import','test-import','test-import:3:r-1','','','','test/reading',1,'sha256:7820b66304d708879603db391dd977cb750036217acb1aae2c0a0573a53cfb83','verbatim','application/json',X'7b226174223a22323032362d31302d30315430373a30303a30305a222c226964223a22722d31222c2276616c7565223a312e357d');
INSERT INTO facet VALUES('400af663-38b9-5076-bec8-4115d97bd64a',1,'test.gone',1.0,'Normal',1,'2026-10-05T07:00:00.000000000Z','','matt','medium','','test-import','test-import','test-import:3:g-1','','','','test/gone',1,'sha256:8a904e65e6c18d125d7d2782a6bbc987f1e55064e8a493988ad4f508d2038953','verbatim','application/json',X'7b226174223a22323032362d31302d30355430373a30303a30305a222c226964223a22672d31222c2276616c7565223a332e357d');
INSERT INTO facet VALUES('3741dcc9-46d1-5b95-84c5-a98e94c3ac53',1,'test.reading',1.0,'Normal',2,'2026-10-02T07:00:00.000000000Z','','matt','medium','','test-import','test-import','test-import:3:r-2','','','','test/reading',1,'sha256:f45ee7a2c8bb8b4d2f5c3f1e2337784a7fdfc54e51244b389b6634af3c31d12a','verbatim','application/json',X'7b226174223a22323032362d31302d30325430373a30303a30305a222c226964223a22722d32222c2276616c7565223a322e357d');
CREATE TABLE anam_event (
  seq                    INTEGER PRIMARY KEY CHECK (seq >= 1),
  kind                   TEXT    NOT NULL,
  facet_id               TEXT    NOT NULL,
  anam_time              TEXT    NOT NULL,
  artifact_version       INTEGER NOT NULL,
  prior_artifact_version INTEGER NOT NULL DEFAULT 0
);
INSERT INTO anam_event VALUES(1,'admitted','ed4af0f9-ae12-5def-8b77-1dfdfcb9f54e','2026-11-02T06:56:27.251076561Z',1,0);
INSERT INTO anam_event VALUES(2,'admitted','15fd02c5-b872-5c78-853d-2128968b8bfa','2026-11-02T06:56:27.251076561Z',1,0);
INSERT INTO anam_event VALUES(3,'admitted','fe61ec2a-6d8b-56b0-8930-e3270f4ad44c','2026-11-02T06:56:27.251076561Z',1,0);
INSERT INTO anam_event VALUES(4,'admitted','2f107449-3dc6-594e-9e52-916eb6008cf5','2026-11-02T06:56:27.251076561Z',1,0);
INSERT INTO anam_event VALUES(5,'admitted','400af663-38b9-5076-bec8-4115d97bd64a','2026-11-02T06:56:27.251076561Z',1,0);
INSERT INTO anam_event VALUES(6,'admitted','3741dcc9-46d1-5b95-84c5-a98e94c3ac53','2026-11-02T06:56:27.287640266Z',1,0);
CREATE TRIGGER ledger_meta_no_update BEFORE UPDATE ON ledger_meta
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: ledger_meta rows are never updated'); END;
CREATE TRIGGER ledger_meta_no_delete BEFORE DELETE ON ledger_meta
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: ledger_meta rows are never deleted'); END;
CREATE TRIGGER ledger_meta_no_replace BEFORE INSERT ON ledger_meta
WHEN EXISTS (SELECT 1 FROM ledger_meta WHERE key = NEW.key)
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: ledger_meta key already pinned'); END;
CREATE TRIGGER facet_no_update BEFORE UPDATE ON facet
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: facet rows are never updated'); END;
CREATE TRIGGER facet_no_delete BEFORE DELETE ON facet
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: facet rows are never deleted'); END;
CREATE TRIGGER facet_no_replace BEFORE INSERT ON facet
WHEN EXISTS (SELECT 1 FROM facet
             WHERE facet_id = NEW.facet_id AND artifact_version = NEW.artifact_version)
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: artifact version already written'); END;
CREATE TRIGGER facet_version_follows BEFORE INSERT ON facet
WHEN NEW.artifact_version > 1
 AND NOT EXISTS (SELECT 1 FROM facet
                 WHERE facet_id = NEW.facet_id AND artifact_version = NEW.artifact_version - 1)
BEGIN SELECT RAISE(ABORT, 'ledger: artifact version does not follow the prior version'); END;
CREATE TRIGGER anam_event_no_update BEFORE UPDATE ON anam_event
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: anam_event rows are never updated'); END;
CREATE TRIGGER anam_event_no_delete BEFORE DELETE ON anam_event
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: anam_event rows are never deleted'); END;
CREATE TRIGGER anam_event_seq_follows BEFORE INSERT ON anam_event
WHEN NEW.seq IS NOT (SELECT COALESCE(MAX(seq), 0) + 1 FROM anam_event)
BEGIN SELECT RAISE(ABORT, 'ledger: anam-event seq does not follow the newest seq'); END;
CREATE INDEX anam_event_by_facet ON anam_event (facet_id, seq);
COMMIT;
