package ops

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/span"
)

// A store written before template_body was named holds its bodies in inline_projection.
// A read-only read finds them under the old name, as a move reads a file this build has
// not opened. Open then moves them across and drops the old table, so a receipt that
// names one still finds it, and the moved row is as immutable as it was.
func TestOpen_MovesInlineProjectionBodiesIntoTemplateBody(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "anam.db")
	body := []byte(`{"max_classification":"Normal","statuses":["admitted"],"types":["test.reading"]}`)
	hash := Digest(body)

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`CREATE TABLE inline_projection (hash TEXT PRIMARY KEY, body BLOB NOT NULL)`,
		`CREATE TRIGGER inline_projection_no_delete BEFORE DELETE ON inline_projection
			BEGIN SELECT RAISE(ABORT, 'inline_projection is immutable'); END`,
	} {
		if _, err := raw.ExecContext(ctx, q); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	if _, err := raw.ExecContext(ctx, `INSERT INTO inline_projection (hash, body) VALUES (?, ?)`, hash, body); err != nil {
		t.Fatal(err)
	}
	// The read-only read needs the other tables a prepared store has.
	if _, err := raw.ExecContext(ctx, schema); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	ro, err := OpenReadOnly(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	reg, err := ro.ReadRegistered(ctx)
	_ = ro.Close()
	if err != nil || len(reg.TemplateBodies) != 1 || reg.TemplateBodies[0].Hash != hash {
		t.Fatalf("read-only read of an unrenamed file = %+v, %v", reg.TemplateBodies, err)
	}

	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 { // the second open finds nothing left to move
		db, err := Open(ctx, path, buckets)
		if err != nil {
			t.Fatalf("Open %d: %v", i+1, err)
		}
		got, err := db.Get(ctx, KindTemplateBody, hash)
		if err != nil || !bytes.Equal(got, body) {
			t.Errorf("open %d: body = %q, %v", i+1, got, err)
		}
		if old, err := hasTable(ctx, db.db, "inline_projection"); err != nil || old {
			t.Errorf("open %d: inline_projection still there: %v, %v", i+1, old, err)
		}
		if _, err := db.db.ExecContext(ctx, `DELETE FROM template_body WHERE hash = ?`, hash); err == nil {
			t.Errorf("open %d: a moved body could be deleted", i+1)
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
