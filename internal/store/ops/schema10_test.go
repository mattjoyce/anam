package ops_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

// Card 260: schema 10 files template bodies in template_body, a file of either version
// is refused by the other build, and a schema-9 file is carried by ReadCarrySource and
// CarrySchema. The schema-9 file in these tests is opstest.Schema9, which the schema-9
// daemon wrote.

// authored is every table a carry writes, as the schema-10 store names it.
var authored = []string{
	"registered_definition", "registered_projection", "template_body", "admission_hold",
	"receipt", "served_value", "saved_selection", "saved_selection_code", "definition",
}

// asSchema9 is a schema-10 table's name in a schema-9 file.
func asSchema9(table string) string {
	if table == "template_body" {
		return "inline_projection"
	}
	return table
}

func schema9(t *testing.T) opstest.Schema9Paths {
	t.Helper()
	p, err := opstest.Schema9(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// fileSum is the SHA-256 of the file at path.
func fileSum(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // a path under the test's own directory
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func objects(t *testing.T, path string) []string {
	t.Helper()
	o, err := opstest.Objects(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

func rows(t *testing.T, path, table string) []string {
	t.Helper()
	r, err := opstest.Rows(context.Background(), path, table)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// sqlExec runs statements against the file at path on a handle of its own, as a hand
// edit or another build would.
func sqlExec(t *testing.T, path string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

// sqlCount runs one COUNT query against the file at path on a handle of its own.
func sqlCount(t *testing.T, path, query string) int {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	var n int
	if err := db.QueryRow(query).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

// Proof 1: a fresh store files template bodies in template_body, which refuses an
// update and a delete for every writer, and has no inline_projection.
func TestSchema10_AFreshStoreFilesTemplateBodiesInTemplateBody(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, path := open(t)
	got := objects(t, path)
	for _, want := range []string{"table template_body", "trigger template_body_no_update", "trigger template_body_no_delete"} {
		if !slices.Contains(got, want) {
			t.Errorf("a fresh store lacks %s", want)
		}
	}
	for _, o := range got {
		if strings.Contains(o, "inline_projection") {
			t.Errorf("a fresh store has %s", o)
		}
	}
	digest, err := d.Put(ctx, ops.KindTemplateBody, []byte(`{"types":["test.weight"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if r := rows(t, path, "template_body"); len(r) != 1 || !strings.HasPrefix(r[0], "hash="+digest) {
		t.Fatalf("template_body holds %v", r)
	}
	for _, stmt := range []string{`UPDATE template_body SET body = x'00' WHERE hash = ?`, `DELETE FROM template_body WHERE hash = ?`} {
		if _, err := d.SQL().ExecContext(ctx, stmt, digest); err == nil || !strings.Contains(err.Error(), "template_body is immutable") {
			t.Errorf("%s = %v, want the trigger's refusal", stmt, err)
		}
	}
}

func TestSchema9Fixture_RunsTheShippedSchema9Text(t *testing.T) {
	t.Parallel()
	ddl, err := opstest.Schema9File("ops.sql")
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(ddl); hex.EncodeToString(sum[:]) != opstest.Schema9DDLSHA256 {
		t.Fatalf("testdata/schema9/ops.sql is not the schema-9 ops.sql as shipped")
	}
}

// Proof 2: this build refuses a schema-9 file on both of its opens, and the refusal
// comes before its schema is applied, so the file's bytes and tables are as they were.
// Open leaves no file beside it. The read-only open, like any SQLite reader of a WAL
// file, leaves an empty -wal and the -shm index beside it: no page is written.
func TestSchema10_RefusesASchema9FileWithoutChangingIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := schema9(t)
	// The tables are listed from a twin, since listing them opens the file read-only.
	sum, objs := fileSum(t, p.Store), objects(t, schema9(t).Store)
	beside := func() map[string]int64 {
		out := map[string]int64{}
		for _, suffix := range []string{"-wal", "-shm"} {
			if fi, err := os.Stat(p.Store + suffix); err == nil {
				out[suffix] = fi.Size()
			}
		}
		return out
	}
	if got := beside(); len(got) != 0 {
		t.Fatalf("the fixture has %v beside it before any open", got)
	}

	_, err := ops.Open(ctx, p.Store, buckets(t))
	if !errors.Is(err, ops.ErrSchemaVersion) || !strings.Contains(err.Error(), "database holds 9, this build writes 10") ||
		!strings.Contains(err.Error(), "move-artifacts --registered-from") {
		t.Fatalf("Open of a schema-9 file = %v, want ErrSchemaVersion naming the carry", err)
	}
	if got := beside(); len(got) != 0 {
		t.Errorf("the refused Open left %v beside the file", got)
	}
	if _, err := ops.OpenReadOnly(ctx, p.Store); !errors.Is(err, ops.ErrSchemaVersion) {
		t.Fatalf("OpenReadOnly of a schema-9 file = %v, want ErrSchemaVersion", err)
	}
	if got := beside(); got["-wal"] != 0 {
		t.Errorf("the refused read-only open wrote %d bytes of WAL", got["-wal"])
	}
	if got := fileSum(t, p.Store); got != sum {
		t.Errorf("the refused opens changed the file's bytes")
	}
	if got := objects(t, p.Store); !slices.Equal(got, objs) {
		t.Errorf("the refused opens changed the file's tables:\n%v\nwas\n%v", got, objs)
	}
}

// Proof 2, the other way: the schema-9 build's open, replayed statement for statement
// on a schema-10 file holding every authored kind, refuses it. Before it refuses, its
// schema adds an empty inline_projection and its two triggers, and nothing else: every
// authored row and every meta key are as they were. This build still opens the file, and
// the carry reads it, ignoring the empty table; a row in it would be refused.
func TestSchema9Build_RefusesASchema10FileAndAddsOnlyAnEmptyTable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, path := open(t)
	body, err := d.Put(ctx, ops.KindTemplateBody, []byte(`{"types":["test.weight"]}`))
	if err != nil {
		t.Fatal(err)
	}
	served, err := d.Put(ctx, ops.KindServed, []byte("a served value"))
	if err != nil {
		t.Fatal(err)
	}
	// The carry follows a receipt's references, so the receipt names rows the file holds.
	rcpt := `{"served_hash":"` + served + `","template_hash":"` + body + `","template_inline":true}`
	if _, err := d.Put(ctx, ops.KindReceipt, []byte(rcpt)); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{ops.KindSelection, ops.KindSelectionCode} {
		if _, err := d.Put(ctx, k, []byte("a "+k)); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.RegisterProjection(ctx, ops.ProjectionRow{Name: "weights", Version: 1, Body: `{}`}, "2026-11-02T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if err := d.RegisterDefinition(ctx, ops.DefinitionRow{Type: "test.reading", Version: 1, Body: `{}`}, "2026-11-02T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if err := d.SetHold(ctx, ops.HoldAll, "2026-11-02T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if err := d.PutDefinitions(ctx, []ops.DefinitionRow{{Type: "test.weight", Version: 7, Body: `{}`}}); err != nil {
		t.Fatal(err)
	}
	if err := d.PinLedger(ctx, "ledger-a"); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	objs := objects(t, path)
	before := map[string][]string{}
	for _, table := range append(slices.Clone(authored), "meta") {
		before[table] = rows(t, path, table)
	}

	if err := opstest.ReplaySchema9Open(ctx, path); !errors.Is(err, opstest.ErrSchema9Refused) || !strings.Contains(err.Error(), "database holds 10") {
		t.Fatalf("the schema-9 open of a schema-10 file = %v, want its refusal", err)
	}
	added := []string{"table inline_projection", "trigger inline_projection_no_delete", "trigger inline_projection_no_update"}
	want := slices.Sorted(slices.Values(append(slices.Clone(objs), added...)))
	if got := objects(t, path); !slices.Equal(got, want) {
		t.Fatalf("after the schema-9 open the file holds\n%v\nwant\n%v", got, want)
	}
	for table, was := range before {
		if got := rows(t, path, table); !slices.Equal(got, was) {
			t.Errorf("%s changed under the schema-9 open:\n%v\nwas\n%v", table, got, was)
		}
	}
	if got := rows(t, path, "inline_projection"); len(got) != 0 {
		t.Errorf("the schema-9 open filed %d rows", len(got))
	}

	again, err := ops.Open(ctx, path, buckets(t))
	if err != nil {
		t.Fatalf("this build after the schema-9 open: %v", err)
	}
	if _, err := again.Get(ctx, ops.KindTemplateBody, body); err != nil {
		t.Errorf("the template body after the schema-9 open: %v", err)
	}
	if err := again.Close(); err != nil {
		t.Fatal(err)
	}
	src, err := ops.ReadCarrySource(ctx, path)
	if err != nil || src.Schema != 10 || len(src.TemplateBodies) != 1 {
		t.Fatalf("ReadCarrySource after the schema-9 open = schema %d, %d bodies, %v", src.Schema, len(src.TemplateBodies), err)
	}
	sqlExec(t, path, `INSERT INTO inline_projection (hash, body) VALUES ('`+ops.Digest([]byte("x"))+`', x'78')`)
	if _, err := ops.ReadCarrySource(ctx, path); !errors.Is(err, ops.ErrCarrySource) || !strings.Contains(err.Error(), "1 rows in inline_projection") {
		t.Fatalf("ReadCarrySource of a schema-10 file with rows under the old name = %v, want ErrCarrySource", err)
	}
}

// The carry reads the schema-9 fixture whole, in one read transaction, without
// writing it: the template bodies come from inline_projection, every row as stored.
func TestReadCarrySource_ReadsTheSchema9Fixture(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := schema9(t)
	sum := fileSum(t, p.Store)
	src, err := ops.ReadCarrySource(ctx, p.Store)
	if err != nil {
		t.Fatal(err)
	}
	if src.Schema != 9 || src.LedgerID == "" || src.BucketWidth == "" {
		t.Errorf("source schema %d, ledger %q, width %q", src.Schema, src.LedgerID, src.BucketWidth)
	}
	stored := rows(t, p.Store, "inline_projection")
	if len(src.TemplateBodies) != len(stored) || len(stored) != 4 {
		t.Fatalf("read %d template bodies, the file holds %d", len(src.TemplateBodies), len(stored))
	}
	for _, b := range src.TemplateBodies {
		if ops.Digest(b.Body) != b.Hash {
			t.Errorf("template body %s does not hash to its key", b.Hash)
		}
	}
	if len(src.Definitions) != 2 || len(src.Projections) != 2 || len(src.Holds) != 2 || len(src.Content) != 13 || len(src.Provisioned) != 8 {
		t.Errorf("read %d registered Definitions, %d projections, %d holds, %d content rows, %d provisioned",
			len(src.Definitions), len(src.Projections), len(src.Holds), len(src.Content), len(src.Provisioned))
	}
	left := map[string]int{}
	for _, c := range src.Left {
		left[c.Table] = c.Rows
	}
	if left["facet_index"] != 6 || left["anam_event_index"] != 6 || left["checkpoint"] == 0 {
		t.Errorf("left behind %v", src.Left)
	}
	if fileSum(t, p.Store) != sum {
		t.Error("the read changed the file")
	}
}

// Proof 5: a source the carry cannot read whole is refused by the read, naming why,
// and is not changed by it.
func TestReadCarrySource_RefusesASourceItCannotCarryWhole(t *testing.T) {
	t.Parallel()
	someHash := ops.Digest([]byte("x"))
	for name, tc := range map[string]struct {
		edit []string
		want error
		says string
	}{
		"schema 8":  {[]string{`UPDATE meta SET value = '8' WHERE key = 'schema_version'`}, ops.ErrCarrySource, `schema version "8"`},
		"schema 11": {[]string{`UPDATE meta SET value = '11' WHERE key = 'schema_version'`}, ops.ErrCarrySource, `schema version "11"`},
		"no schema": {[]string{`DELETE FROM meta WHERE key = 'schema_version'`}, ops.ErrCarrySource, `schema version ""`},
		"no template bodies": {[]string{`DROP TABLE inline_projection`}, ops.ErrCarrySource,
			"holds its template bodies in inline_projection, and this file has no such table"},
		"rows under the new name": {[]string{
			`CREATE TABLE template_body (hash TEXT PRIMARY KEY, body BLOB NOT NULL)`,
			`INSERT INTO template_body VALUES ('` + someHash + `', x'78')`,
		}, ops.ErrCarrySource, "1 rows in template_body"},
		"an unknown meta key": {[]string{`INSERT INTO meta VALUES ('owner', 'someone')`}, ops.ErrCarrySource, "owner"},
		"a corrupt template body": {[]string{
			`INSERT OR REPLACE INTO inline_projection (hash, body) SELECT hash, x'00' FROM inline_projection ORDER BY hash LIMIT 1`,
		}, ops.ErrCorrupt, "template-body sha256:"},
		"a corrupt receipt": {[]string{
			`INSERT OR REPLACE INTO receipt (hash, body) SELECT hash, x'00' FROM receipt ORDER BY hash LIMIT 1`,
		}, ops.ErrCorrupt, "receipt sha256:"},
		"a receipt's served value missing": {[]string{
			`DROP TRIGGER served_value_no_delete`,
			`DELETE FROM served_value WHERE hash = (SELECT json_extract(CAST(body AS TEXT), '$.served_hash') FROM receipt ORDER BY hash LIMIT 1)`,
		}, ops.ErrCorrupt, "names served value"},
		"an inline receipt's template body missing": {[]string{
			`DROP TRIGGER inline_projection_no_delete`,
			`DELETE FROM inline_projection WHERE hash IN (SELECT json_extract(CAST(body AS TEXT), '$.template_hash') FROM receipt WHERE json_extract(CAST(body AS TEXT), '$.template_inline'))`,
		}, ops.ErrCorrupt, "names inline template body"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := schema9(t)
			sqlExec(t, p.Store, tc.edit...)
			sum := fileSum(t, p.Store)
			_, err := ops.ReadCarrySource(context.Background(), p.Store)
			if !errors.Is(err, tc.want) || !strings.Contains(err.Error(), tc.says) {
				t.Fatalf("ReadCarrySource = %v, want %v saying %q", err, tc.want, tc.says)
			}
			if fileSum(t, p.Store) != sum {
				t.Error("the refused read changed the file")
			}
		})
	}

	// A catalog receipt minted before card 257 filed no template body; verify replays it
	// from the catalog. Removing a catalog receipt's body imitates that, and the read
	// still accepts the file.
	t.Run("a catalog receipt without its body", func(t *testing.T) {
		t.Parallel()
		p := schema9(t)
		all := sqlCount(t, p.Store, `SELECT COUNT(*) FROM inline_projection`)
		sqlExec(t, p.Store,
			`DROP TRIGGER inline_projection_no_delete`,
			`DELETE FROM inline_projection WHERE hash IN (SELECT json_extract(CAST(body AS TEXT), '$.template_hash') FROM receipt WHERE NOT coalesce(json_extract(CAST(body AS TEXT), '$.template_inline'), 0))
			   AND hash NOT IN (SELECT json_extract(CAST(body AS TEXT), '$.template_hash') FROM receipt WHERE json_extract(CAST(body AS TEXT), '$.template_inline'))`,
		)
		before := sqlCount(t, p.Store, `SELECT COUNT(*) FROM inline_projection`)
		if before >= all {
			t.Fatal("the fixture has no catalog receipt body to remove")
		}
		src, err := ops.ReadCarrySource(context.Background(), p.Store)
		if err != nil {
			t.Fatalf("ReadCarrySource = %v, want the file accepted", err)
		}
		if len(src.TemplateBodies) != before {
			t.Fatalf("read %d template bodies, the file holds %d", len(src.TemplateBodies), before)
		}
	})

	t.Run("no file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "tracking.db")
		if _, err := ops.ReadCarrySource(context.Background(), path); err == nil {
			t.Fatal("ReadCarrySource of no file succeeded")
		}
		if _, err := os.Stat(path); err == nil {
			t.Error("the read made the file it was asked to read")
		}
	})
}

// carried writes the fixture's authored rows into a fresh store and returns its path.
func carried(t *testing.T, p opstest.Schema9Paths) (*ops.DB, string) {
	t.Helper()
	src, err := ops.ReadCarrySource(context.Background(), p.Store)
	if err != nil {
		t.Fatal(err)
	}
	d, path := open(t)
	if err := d.CarrySchema(context.Background(), src.Registered); err != nil {
		t.Fatal(err)
	}
	return d, path
}

// Proof 3, at the store: every row the schema-9 file authored is in the new store as it
// was, bytes, keys and registration times, the template bodies moved from
// inline_projection to template_body, and the provisioned Definitions with them.
func TestCarrySchema_EveryAuthoredRowArrivesAsItWas(t *testing.T) {
	t.Parallel()
	p := schema9(t)
	_, path := carried(t, p)
	for _, table := range authored {
		was, got := rows(t, p.Store, asSchema9(table)), rows(t, path, table)
		if len(was) == 0 {
			t.Errorf("the fixture holds no %s rows to carry", asSchema9(table))
		}
		if !slices.Equal(got, was) {
			t.Errorf("%s after the carry:\n%v\n%s before it:\n%v", table, got, asSchema9(table), was)
		}
	}
	for _, table := range []string{"checkpoint", "checkpoint_value", "facet_index"} {
		if got := rows(t, path, table); len(got) != 0 {
			t.Errorf("the carry wrote %d %s rows; they are derived", len(got), table)
		}
	}
}

// Proof 5: the carry is one transaction into a fresh store. A failure part way through
// writes nothing, and the same carry then succeeds; a second carry into the same store
// is refused and leaves it as it was, as is a carry into a store a daemon provisioned.
func TestCarrySchema_IsAllOrNothingAndOnlyIntoAFreshStore(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p := schema9(t)
	src, err := ops.ReadCarrySource(ctx, p.Store)
	if err != nil {
		t.Fatal(err)
	}
	d, path := open(t)

	// The last template body twice: every row before it is written inside the
	// transaction, then the duplicate key fails it.
	broken := src.Registered
	broken.TemplateBodies = append(slices.Clone(broken.TemplateBodies), broken.TemplateBodies[len(broken.TemplateBodies)-1])
	if err := d.CarrySchema(ctx, broken); err == nil {
		t.Fatal("a carry with a duplicate key succeeded")
	}
	// A corrupt row after every other.
	corrupt := src.Registered
	corrupt.Content = append(slices.Clone(corrupt.Content), ops.Content{Kind: ops.KindReceipt, Hash: someDigest, Body: []byte("other")})
	if err := d.CarrySchema(ctx, corrupt); !errors.Is(err, ops.ErrCorrupt) {
		t.Fatalf("a carry with a corrupt row = %v, want ErrCorrupt", err)
	}
	for _, table := range authored {
		if got := rows(t, path, table); len(got) != 0 {
			t.Fatalf("a failed carry left %d %s rows", len(got), table)
		}
	}

	if err := d.CarrySchema(ctx, src.Registered); err != nil {
		t.Fatalf("the carry after the failures: %v", err)
	}
	after := map[string][]string{}
	for _, table := range authored {
		after[table] = rows(t, path, table)
	}
	err = d.CarrySchema(ctx, src.Registered)
	if !errors.Is(err, ops.ErrCarryTarget) || !strings.Contains(err.Error(), "4 template_body") {
		t.Fatalf("a second carry = %v, want ErrCarryTarget naming what the store holds", err)
	}
	for _, table := range authored {
		if got := rows(t, path, table); !slices.Equal(got, after[table]) {
			t.Errorf("the refused second carry changed %s", table)
		}
	}

	provisioned, _ := open(t)
	if err := provisioned.PutDefinitions(ctx, []ops.DefinitionRow{{Type: "test.weight", Version: 7, Body: `{}`}}); err != nil {
		t.Fatal(err)
	}
	if err := provisioned.CarrySchema(ctx, src.Registered); !errors.Is(err, ops.ErrCarryTarget) || !strings.Contains(err.Error(), "1 definition") {
		t.Fatalf("a carry into a provisioned store = %v, want ErrCarryTarget", err)
	}
}

var someDigest = ops.Digest([]byte("some bytes"))

// Proof 6: a backup of a carried store holds every template body and authored row, and
// opens as the store it was taken from.
func TestBackupTo_KeepsTheTemplateBodiesAndEveryAuthoredRow(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, path := carried(t, schema9(t))
	backup := filepath.Join(t.TempDir(), "tracking.db")
	if err := d.BackupTo(ctx, backup); err != nil {
		t.Fatal(err)
	}
	for _, table := range authored {
		if got, was := rows(t, backup, table), rows(t, path, table); !slices.Equal(got, was) || len(was) == 0 {
			t.Errorf("%s in the backup:\n%v\nin the store:\n%v", table, got, was)
		}
	}
	restored, err := ops.Open(ctx, backup, buckets(t))
	if err != nil {
		t.Fatalf("open the backup as the store: %v", err)
	}
	defer func() { _ = restored.Close() }()
	bodies, err := restored.List(ctx, ops.KindTemplateBody)
	if err != nil || len(bodies) != 4 {
		t.Fatalf("the restored store lists %d template bodies (%v)", len(bodies), err)
	}
	for _, h := range bodies {
		if _, err := restored.Get(ctx, ops.KindTemplateBody, h); err != nil {
			t.Errorf("template body %s in the restored store: %v", h, err)
		}
	}
}
