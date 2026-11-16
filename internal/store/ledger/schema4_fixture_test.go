package ledger_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	_ "embed"
	"fmt"
	"path/filepath"
	"testing"
)

// schemaV3 is the byte-for-byte schema shipped before card 270b, not a schema
// reconstructed from the current one. Its pin prevents a new schema from silently
// changing the starting point of the carry tests.
//
//go:embed testdata/schema3/ledger.sql
var schemaV3 string

func TestSchema4_Schema3FixtureIsPinned(t *testing.T) {
	t.Parallel()
	const want = "906d1cda1bdc769932ef7cc8a9c09ed2845ee5c6437b9c943b007a553e738ea0"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(schemaV3))); got != want {
		t.Fatalf("schema 3 fixture hash = %s, want %s", got, want)
	}
}

func buildV3(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ledger.db")
	db := rawDB(t, path)
	for _, text := range []string{schemaV3, `
INSERT INTO ledger_meta VALUES
  ('schema_version', '3'), ('person_id', 'test-person'), ('ledger_id', 'v3-ledger'),
  ('source-note', 'keep this pin too');
INSERT INTO facet VALUES
  ('f1', 1, 'health.blood-pressure', 0.9, 'Normal', 2,
   '2026-07-05T06:30:00.000000000Z', 'arm:left', 'test-person', 'high', 'morning',
   'bp-csv', 'bp-injector', 'bp-csv:1', 'method', 'model', 'watermark',
   'bp-csv/reading', 1, 'sha256:first', 'verbatim', 'application/json', X'7B7D'),
  ('f1', 2, 'health.blood-pressure', 1.0, 'Sensitive', 3,
   '2026-07-05T06:30:00.000000000Z', 'arm:left', 'test-person', 'medium', 'corrected',
   'bp-csv', 'bp-injector', 'bp-csv:1', '', '', '',
   'bp-csv/reading', 1, 'sha256:second', 'verbatim', 'application/json', X'7B2261223A327D'),
  ('f2', 1, 'record.note', 0.7, 'Normal', 1,
   '2026-07-06T06:30:00.000000000Z', '', 'test-person', 'high', '',
   'notes', 'note-injector', 'notes:2', '', '', '',
   'note', 1, 'sha256:third', 'verbatim', 'text/plain', X'006E6F7465FF');
INSERT INTO anam_event VALUES
  (1, 'admitted', 'f1', '2026-07-10T09:00:00.000000000Z', 1, 0),
  (2, 'superseded', 'f1', '2026-07-10T09:01:00.000000000Z', 2, 1),
  (3, 'admitted', 'f2', '2026-07-10T09:02:00.000000000Z', 1, 0),
  (4, 'tombstoned', 'f2', '2026-07-10T09:03:00.000000000Z', 1, 0);
CREATE INDEX IF NOT EXISTS anam_event_by_facet ON anam_event (facet_id, seq);
`} {
		if _, err := db.ExecContext(context.Background(), text); err != nil {
			t.Fatalf("build schema 3: %v", err)
		}
	}
	return path
}

// ledgerState reads every value by column position, including payload bytes, and
// every sqlite_master object's SQL (tables, triggers and indexes, including NULL
// SQL on automatic indexes). Physical root pages are not part of schema SQL.
func ledgerState(t *testing.T, db *sql.DB) map[string][][]any {
	t.Helper()
	state := make(map[string][][]any)
	for name, query := range map[string]string{
		"facets": `SELECT * FROM facet ORDER BY facet_id, artifact_version`,
		"events": `SELECT * FROM anam_event ORDER BY seq`,
		"pins":   `SELECT * FROM ledger_meta ORDER BY key`,
		"W":      `SELECT COALESCE(MAX(seq), 0) FROM anam_event`,
		"schema": `SELECT type, name, tbl_name, sql FROM sqlite_master ORDER BY type, name`,
	} {
		state[name] = queryValues(t, db, query)
	}
	return state
}

func queryValues(t *testing.T, db *sql.DB, query string, args ...any) [][]any {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), query, args...)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	var result [][]any
	for rows.Next() {
		values, dest := make([]any, len(cols)), make([]any, len(cols))
		for i := range dest {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			t.Fatal(err)
		}
		result = append(result, values)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return result
}
