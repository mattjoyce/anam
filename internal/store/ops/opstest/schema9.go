package opstest

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// The schema-9 fixture (card 260) is a ledger and an operational store the schema-9
// daemon at 6abb9d7 wrote itself, kept as SQL dumps of the two files, with the catalog
// it ran on and what it answered (expect.json). testdata/schema9/generate_test.go.txt is
// the code that made it and says how it was run. It holds receipts through an inline
// projection, a catalog template and two registered template versions, their template
// bodies under the old table name inline_projection, three saved selections of which two
// share a code's first eight digits, two registered Definition versions, two registered
// projection versions, a hold on one source and a hold on all, and a provisioned
// Definition (test.gone) a later catalog drops.
//
// testdata/schema9/ops.sql is the schema-9 ops.sql as shipped, byte for byte
// (Schema9DDLSHA256 pins it), which ReplaySchema9Open runs.

//go:embed testdata/schema9
var schema9 embed.FS

// Schema9DDLSHA256 is the SHA-256 of ops.sql at schema version 9, the pin the schema
// guard held before card 260 bumped it.
const Schema9DDLSHA256 = "f43f31b88a06b17f64be378c3cd32aa9465867e4f3972e13b676a329b4ef5702"

// Schema9Paths is where Schema9 wrote the fixture.
type Schema9Paths struct {
	Ledger, Store            string
	Definitions, Projections string
}

// Schema9 writes the fixture into dir, made if absent and otherwise empty. It restores
// ledger.db and tracking.db from their dumps and copies the catalog under catalog/.
func Schema9(ctx context.Context, dir string) (Schema9Paths, error) {
	p := Schema9Paths{
		Ledger: filepath.Join(dir, "ledger.db"), Store: filepath.Join(dir, "tracking.db"),
		Definitions: filepath.Join(dir, "catalog", "definitions"), Projections: filepath.Join(dir, "catalog", "projections"),
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return Schema9Paths{}, fmt.Errorf("opstest: fixture directory: %w", err)
	}
	for file, dump := range map[string]string{p.Ledger: "ledger.sql", p.Store: "tracking.sql"} {
		if err := restore(ctx, file, dump); err != nil {
			return Schema9Paths{}, err
		}
	}
	sub, err := fs.Sub(schema9, "testdata/schema9/catalog")
	if err != nil {
		return Schema9Paths{}, fmt.Errorf("opstest: fixture catalog: %w", err)
	}
	if err := os.CopyFS(filepath.Join(dir, "catalog"), sub); err != nil {
		return Schema9Paths{}, fmt.Errorf("opstest: copy the fixture catalog: %w", err)
	}
	return p, nil
}

// Schema9File returns one file of the fixture's testdata by its name there, such as
// expect.json or ops.sql.
func Schema9File(name string) ([]byte, error) {
	b, err := schema9.ReadFile(path.Join("testdata/schema9", name))
	if err != nil {
		return nil, fmt.Errorf("opstest: fixture %s: %w", name, err)
	}
	return b, nil
}

// restore makes the SQLite file at file from the dump of that name.
func restore(ctx context.Context, file, dump string) error {
	text, err := Schema9File(dump)
	if err != nil {
		return err
	}
	if _, err := os.Stat(file); err == nil {
		return fmt.Errorf("opstest: %s already exists", file)
	}
	db, err := sql.Open("sqlite", file)
	if err != nil {
		return fmt.Errorf("opstest: open %s: %w", file, err)
	}
	if _, err := db.ExecContext(ctx, string(text)); err != nil {
		_ = db.Close()
		return fmt.Errorf("opstest: restore %s: %w", dump, err)
	}
	// The daemon keeps both files in WAL mode; a restored file is put back in it.
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		_ = db.Close()
		return fmt.Errorf("opstest: restore %s: %w", dump, err)
	}
	return db.Close()
}

// ErrSchema9Refused is what ReplaySchema9Open returns for a file whose version is not 9,
// standing for the shipped build's ops.ErrSchemaVersion.
var ErrSchema9Refused = errors.New("opstest: the schema-9 build refuses this file")

// ReplaySchema9Open does to the file at path what ops.Open at schema version 9 did,
// statement for statement (6abb9d7, internal/store/ops/ops.go). It opens with the same
// pragmas, applies the shipped schema-9 ops.sql, requires snapshot_part's digest column
// (the pre-card-107 step is not replayed), then records version 9 if no version is
// recorded and refuses any other. It
// stands in for running the older binary against a schema-10 file, so a test can show
// what that binary changes in the file before it refuses it.
func ReplaySchema9Open(ctx context.Context, path string) error {
	ddl, err := Schema9File("ops.sql")
	if err != nil {
		return err
	}
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("opstest: open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.ExecContext(ctx, string(ddl)); err != nil {
		return fmt.Errorf("opstest: apply the schema-9 schema: %w", err)
	}
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pragma_table_info('snapshot_part') WHERE name = 'digest'`).Scan(&n); err != nil {
		return fmt.Errorf("opstest: snapshot_part columns: %w", err)
	}
	if n == 0 {
		return errors.New("opstest: the file has no snapshot_part digest; the replay does not cover the pre-card-107 step")
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO meta (key, value) VALUES ('schema_version', '9') ON CONFLICT (key) DO NOTHING`); err != nil {
		return fmt.Errorf("opstest: pin schema version: %w", err)
	}
	var stored string
	if err := db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&stored); err != nil {
		return fmt.Errorf("opstest: read schema version: %w", err)
	}
	if stored != "9" {
		return fmt.Errorf("%w: database holds %s, this build writes 9", ErrSchema9Refused, stored)
	}
	return nil
}

// Objects lists the schema objects of the SQLite file at path, as "type name", sorted:
// what a test compares to say which tables, indexes and triggers an open added.
func Objects(ctx context.Context, path string) ([]string, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("opstest: open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(ctx, `SELECT type, name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, fmt.Errorf("opstest: list %s: %w", path, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var typ, name string
		if err := rows.Scan(&typ, &name); err != nil {
			return nil, fmt.Errorf("opstest: list %s: %w", path, err)
		}
		out = append(out, typ+" "+name)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("opstest: list %s: %w", path, err)
	}
	sort.Strings(out)
	return out, nil
}

// rowsOf reads every row of a table Rows may be asked for. The names are fixed here, so
// no query is made from a caller's string.
var rowsOf = func() map[string]string {
	out := map[string]string{}
	for _, t := range []string{
		"registered_definition", "registered_projection", "inline_projection", "template_body",
		"admission_hold", "receipt", "served_value", "saved_selection", "saved_selection_code",
		"definition", "meta", "checkpoint", "checkpoint_value", "facet_index", "anam_event_index",
	} {
		out[t] = "SELECT * FROM " + t
	}
	return out
}()

// Rows reads every row of table in the SQLite file at path, each row its columns'
// values written out in column order, sorted, which a test compares to say two files
// hold the same rows, bytes and times included.
func Rows(ctx context.Context, path, table string) ([]string, error) {
	query, ok := rowsOf[table]
	if !ok {
		return nil, fmt.Errorf("opstest: Rows does not read %q", table)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return nil, fmt.Errorf("opstest: open %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("opstest: read %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	cols, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("opstest: read %s: %w", table, err)
	}
	var out []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("opstest: read %s: %w", table, err)
		}
		parts := make([]string, len(vals))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				parts[i] = fmt.Sprintf("%s=x'%x'", cols[i], b)
			} else {
				parts[i] = fmt.Sprintf("%s=%v", cols[i], v)
			}
		}
		out = append(out, strings.Join(parts, " "))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("opstest: read %s: %w", table, err)
	}
	sort.Strings(out)
	return out, nil
}
