package ops

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/span"
)

// A store written before card 107 has snapshot_part without a digest. Open adds the
// column and clears the snapshot tables: a part recorded without a digest cannot be
// told from a changed one, and the tier is disposable. A store opened twice is left
// alone the second time.
func TestOpen_BindsSnapshotPartsOfAnOlderStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "anam.db")
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE snapshot (id TEXT PRIMARY KEY, source TEXT NOT NULL, types TEXT NOT NULL, parts INTEGER NOT NULL,
			started_at TEXT NOT NULL, started_w INTEGER NOT NULL, completed_at TEXT, completed_w INTEGER)`,
		`CREATE TABLE snapshot_part (id TEXT NOT NULL, part INTEGER NOT NULL, PRIMARY KEY (id, part))`,
		`CREATE TABLE snapshot_facet (id TEXT NOT NULL, facet_id TEXT NOT NULL, PRIMARY KEY (id, facet_id))`,
		`INSERT INTO snapshot VALUES ('old', 'withings', 'health.measurement', 1, '2026-10-10T09:00:00Z', 1, '2026-10-10T09:00:00Z', 1)`,
		`INSERT INTO snapshot_part VALUES ('old', 1)`,
		`INSERT INTO snapshot_facet VALUES ('old', 'f1')`,
	} {
		if _, err := raw.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}

	db, err := Open(ctx, path, buckets)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	if _, ok, err := db.Snapshot(ctx, "old"); err != nil || ok {
		t.Errorf("the old snapshot survived: held %v, %v", ok, err)
	}
	if c, err := db.SnapshotFacets(ctx, "old"); err != nil || len(c) != 0 {
		t.Errorf("the old snapshot's facets survived: %v, %v", c, err)
	}
	// The column is there to use, and a second open leaves what it holds.
	if _, err := db.RecordSnapshotPart(ctx, SnapshotPart{ID: "new", Source: "withings", Types: []string{"health.measurement"},
		Part: 1, Parts: 1, Digest: "d", FacetIDs: []string{"f2"}}, time.Date(2026, 10, 10, 9, 0, 0, 0, time.UTC), 2); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(ctx, path, buckets)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	if d, ok, err := db.SnapshotPartDigest(ctx, "new", 1); err != nil || !ok || d != "d" {
		t.Errorf("after a second open, part digest = %q, %v, %v", d, ok, err)
	}
}
