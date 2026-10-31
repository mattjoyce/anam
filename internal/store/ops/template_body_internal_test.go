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
// not opened. Open copies them across and leaves the old table, still immutable, for an
// older build to read. A body that older build files afterwards is read by a move at
// once and copied by the next Open, so no receipt loses the body it names.
func TestOpen_CopiesInlineProjectionBodiesIntoTemplateBody(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "anam.db")
	body := []byte(`{"max_classification":"Normal","statuses":["admitted"],"types":["test.reading"]}`)
	hash := Digest(body)
	later := []byte(`{"max_classification":"Normal","statuses":["admitted"],"types":["test.later"]}`)
	laterHash := Digest(later)

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

	readBodies := func(want ...string) {
		t.Helper()
		ro, err := OpenReadOnly(ctx, path)
		if err != nil {
			t.Fatal(err)
		}
		reg, err := ro.ReadRegistered(ctx)
		_ = ro.Close()
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, b := range reg.TemplateBodies {
			got = append(got, b.Hash)
		}
		if len(got) != len(want) {
			t.Fatalf("read-only read = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("read-only read = %v, want %v", got, want)
			}
		}
	}
	readBodies(hash)

	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	open := func(n int, hashes map[string][]byte) {
		t.Helper()
		db, err := Open(ctx, path, buckets)
		if err != nil {
			t.Fatalf("Open %d: %v", n, err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
		}()
		for h, b := range hashes {
			got, err := db.Get(ctx, KindTemplateBody, h)
			if err != nil || !bytes.Equal(got, b) {
				t.Errorf("open %d: body %s = %q, %v", n, h, got, err)
			}
		}
		if old, err := hasTable(ctx, db.db, "inline_projection"); err != nil || !old {
			t.Errorf("open %d: inline_projection gone, so an older build loses its bodies: %v, %v", n, old, err)
		}
		for _, table := range []string{"template_body", "inline_projection"} {
			if _, err := db.db.ExecContext(ctx, `DELETE FROM `+table+` WHERE hash = ?`, hash); err == nil {
				t.Errorf("open %d: a body could be deleted from %s", n, table)
			}
		}
	}
	open(1, map[string][]byte{hash: body})
	open(2, map[string][]byte{hash: body}) // the second open finds nothing new to copy

	// An older build, opened on the file again, files a body under the old name.
	raw, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.ExecContext(ctx, `INSERT INTO inline_projection (hash, body) VALUES (?, ?)`, laterHash, later); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	want := []string{hash, laterHash}
	if want[1] < want[0] {
		want[0], want[1] = want[1], want[0]
	}
	readBodies(want...)
	open(3, map[string][]byte{hash: body, laterHash: later})
}
