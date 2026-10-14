package ledger_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ledger"
)

// schemaV1 is the ledger as the first build wrote it: the facet table without the
// series column, the version pinned at 1. A migration test needs the file an older
// binary left behind, not one this build can make.
const schemaV1 = `
CREATE TABLE ledger_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL);
CREATE TABLE facet (
  facet_id TEXT NOT NULL, artifact_version INTEGER NOT NULL CHECK (artifact_version >= 1),
  type TEXT NOT NULL, confidence REAL NOT NULL, classification TEXT NOT NULL,
  definition_version INTEGER NOT NULL, fact_time TEXT NOT NULL,
  binding_person_id TEXT NOT NULL, binding_assurance TEXT NOT NULL,
  binding_span_hint TEXT NOT NULL DEFAULT '',
  prov_source TEXT NOT NULL, prov_injector TEXT NOT NULL, prov_source_ref TEXT NOT NULL,
  prov_method TEXT NOT NULL DEFAULT '', prov_model TEXT NOT NULL DEFAULT '',
  prov_watermark TEXT NOT NULL DEFAULT '',
  source_schema TEXT NOT NULL, schema_version INTEGER NOT NULL, content_hash TEXT NOT NULL,
  payload_kind TEXT NOT NULL, media_type TEXT NOT NULL, payload BLOB NOT NULL,
  PRIMARY KEY (facet_id, artifact_version)
);
CREATE TABLE anam_event (
  seq INTEGER PRIMARY KEY CHECK (seq >= 1), kind TEXT NOT NULL, facet_id TEXT NOT NULL,
  anam_time TEXT NOT NULL, artifact_version INTEGER NOT NULL,
  prior_artifact_version INTEGER NOT NULL DEFAULT 0
);
CREATE TRIGGER ledger_meta_no_update BEFORE UPDATE ON ledger_meta
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: ledger_meta rows are never updated'); END;
CREATE TRIGGER facet_no_update BEFORE UPDATE ON facet
BEGIN SELECT RAISE(ABORT, 'ledger is append-only: facet rows are never updated'); END;
INSERT INTO ledger_meta VALUES ('schema_version', '1'), ('person_id', 'test-person'), ('ledger_id', 'v1-ledger');
INSERT INTO facet VALUES ('f1', 1, 'health.blood-pressure', 1.0, 'Normal', 1,
  '2026-07-05T06:30:00.000000000Z', 'test-person', 'high', '',
  'bp-csv', 'bp-csv', 'bp-csv:1', '', '', '',
  'bp-csv/reading', 1, 'sha256:x', 'verbatim', 'application/json', X'7B7D');
INSERT INTO anam_event VALUES (1, 'admitted', 'f1', '2026-07-10T09:00:00.000000000Z', 1, 0);
`

func buildV1(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.db")
	old := rawDB(t, path)
	if _, err := old.ExecContext(context.Background(), schemaV1); err != nil {
		t.Fatalf("build the v1 ledger: %v", err)
	}
	return path
}

// An old ledger is refused by every opener until it is migrated, and the refusal
// says migrate, never delete.
func TestOpen_RefusesAnUnmigratedLedger(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := buildV1(t)

	w, err := ledger.Open(ctx, path, person)
	closeIfOpen(w)
	if !errors.Is(err, ledger.ErrSchemaMismatch) {
		t.Fatalf("Open on schema 1 = %v, want ErrSchemaMismatch", err)
	}
	r, err := ledger.OpenReadOnly(ctx, path, person)
	closeIfOpen(r)
	if !errors.Is(err, ledger.ErrSchemaMismatch) {
		t.Fatalf("OpenReadOnly on schema 1 = %v, want ErrSchemaMismatch", err)
	}
}

// Create (init) cannot carry a v1 or v2 ledger forward: schema 3 changed what the
// rows mean (how ids compose, where fact-time lands), not their shape. The refusal
// names the remedy, a new ledger from the sources, and leaves the file exactly as it
// was: no step of the path is applied when a later step cannot be.
func TestCreate_RefusesToCarryAnOldLedgerForward(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := buildV1(t)

	l, err := ledger.Create(ctx, path, person)
	closeIfOpen(l)
	if !errors.Is(err, ledger.ErrSchemaMismatch) {
		t.Fatalf("Create on a v1 ledger = %v, want ErrSchemaMismatch", err)
	}
	for _, want := range []string{"anam init", "resend its source whole", "stays the record"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not say %q", err, want)
		}
	}

	var created, migrated string
	db := rawDB(t, path)
	if err := db.QueryRowContext(ctx, `SELECT value FROM ledger_meta WHERE key = 'schema_version'`).Scan(&created); err != nil || created != "1" {
		t.Errorf("created-at pin = %q, %v; want it untouched at 1", created, err)
	}
	err = db.QueryRowContext(ctx, `SELECT value FROM ledger_meta WHERE key = 'schema_migration:2'`).Scan(&migrated)
	if err == nil {
		t.Errorf("migration to 2 was applied (%q) on a path that cannot reach 3", migrated)
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM facet`).Scan(&n); err != nil || n != 1 {
		t.Errorf("facet rows after refusal = %d, %v; want the 1 the file held", n, err)
	}
}

func TestOpen_RefusesALedgerAheadOfThisBuild(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := buildV1(t)
	db := rawDB(t, path)
	if _, err := db.ExecContext(ctx, `INSERT INTO ledger_meta VALUES ('schema_migration:99', 'later')`); err != nil {
		t.Fatalf("pin a future migration: %v", err)
	}
	l, err := ledger.Create(ctx, path, person)
	closeIfOpen(l)
	if !errors.Is(err, ledger.ErrSchemaMismatch) {
		t.Fatalf("Create on a future schema = %v, want ErrSchemaMismatch", err)
	}
}
