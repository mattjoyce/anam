package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The schema carry (card 260). A schema bump refuses the old file, and anam
// move-artifacts --registered-from copies what the old file authored into a fresh store
// of this version. The old file may be of a version this build's Open refuses, so the
// carry reads it through ReadCarrySource, which knows the supported versions' names for
// what it copies and nothing else. It never writes the old file, and the daemon's
// version guard is not loosened by it: Open and OpenReadOnly still refuse that file.

// ErrCarrySource is returned by ReadCarrySource for a file the carry cannot read as a
// whole, such as an unsupported version, a table missing, rows under a name its version
// does not have, or bookkeeping it does not know where to put.
var ErrCarrySource = errors.New("ops: not a store the carry can read")

// ErrCarryTarget is returned by CarrySchema for a store that is not fresh. It already
// holds authored rows or provisioned Definitions, so a carry into it could not say
// which rows were the old file's.
var ErrCarryTarget = errors.New("ops: the store to carry into is not fresh")

// carryMeta is the meta keys a carry knows, and none is copied. schema_version is this
// build's, bucket_width is pinned by Open from the config, and ledger_id is pinned by
// the first catch-up from the ledger the daemon follows. A key outside it is
// bookkeeping the carry would lose, so the source is refused.
var carryMeta = map[string]bool{"schema_version": true, "bucket_width": true, "ledger_id": true}

// leftBehind are the tables a carry does not copy, by tier. The derived ones are rebuilt
// by replay and minting, the disposable snapshot tables are remade by the next resend,
// and no code of this build reads or writes sync_cursor or token_scope (the importers
// file is read at start instead). Their row counts are reported so a
// count the operator did not expect is seen, not dropped unread.
var leftBehind = []string{
	"facet_index", "anam_event_index", "facet_edge", "replay_cursor",
	"checkpoint", "checkpoint_value", "mint_run", "checkpoint_size",
	"snapshot", "snapshot_part", "snapshot_facet",
	"sync_cursor", "token_scope",
}

// leftBehindCount counts one table of leftBehind. The names are constants, never input.
var leftBehindCount = func() map[string]string {
	out := make(map[string]string, len(leftBehind))
	for _, t := range leftBehind {
		out[t] = "SELECT COUNT(*) FROM " + t
	}
	return out
}()

// TableCount is how many rows one table holds.
type TableCount struct {
	Table string
	Rows  int
}

// CarrySource is an older operational store as the carry read it, with its schema
// version, what it authored, and what it holds that the carry leaves behind.
type CarrySource struct {
	Schema int
	Registered
	// LedgerID and BucketWidth are what the file's meta pinned. Neither is copied; they
	// are reported so the operator can see the new store is about to follow the same
	// ledger.
	LedgerID, BucketWidth string
	// Left is the row count of each table the carry does not copy, in leftBehind order.
	Left []TableCount
}

// ReadCarrySource reads the operational store file at path for a schema carry, on a
// handle SQLite will not accept a write from, in one read transaction. It reads a file
// of version 9 or of this version, and a version-9 file's template bodies under their
// old name, inline_projection. Every content row is checked against the digest that
// keys it here, so a damaged file is refused before anything is written anywhere.
func ReadCarrySource(ctx context.Context, path string) (CarrySource, error) {
	db, err := openReadOnly(ctx, path)
	if err != nil {
		return CarrySource{}, err
	}
	defer func() { _ = db.Close() }()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return CarrySource{}, fmt.Errorf("ops: read %s for the carry: %w", path, err)
	}
	defer func() { _ = tx.Rollback() }() // a read transaction: nothing to commit

	src, err := readCarrySource(ctx, tx)
	if err != nil {
		return CarrySource{}, fmt.Errorf("ops: read %s for the carry: %w", path, err)
	}
	return src, nil
}

func readCarrySource(ctx context.Context, tx *sql.Tx) (CarrySource, error) {
	stored, err := storedSchemaVersion(ctx, tx)
	if err != nil {
		return CarrySource{}, err
	}
	version, err := strconv.Atoi(stored)
	bodies, supported := templateBodies[version]
	if err != nil || !supported {
		return CarrySource{}, fmt.Errorf("%w: it records schema version %q; the carry reads versions 9 and %d",
			ErrCarrySource, stored, schemaVersion)
	}
	tables, err := tableNames(ctx, tx)
	if err != nil {
		return CarrySource{}, err
	}
	if !tables[bodies] {
		return CarrySource{}, fmt.Errorf("%w: a version %d store holds its template bodies in %s, and this file has no such table",
			ErrCarrySource, version, bodies)
	}
	if err := checkOtherBodies(ctx, tx, version, tables); err != nil {
		return CarrySource{}, err
	}
	src := CarrySource{Schema: version}
	if src.LedgerID, src.BucketWidth, err = readCarryMeta(ctx, tx); err != nil {
		return CarrySource{}, err
	}
	if src.Registered, err = readRegistered(ctx, tx, bodies); err != nil {
		return CarrySource{}, err
	}
	if err := src.check(); err != nil {
		return CarrySource{}, err
	}
	for _, t := range leftBehind {
		c := TableCount{Table: t}
		if tables[t] {
			if err := tx.QueryRowContext(ctx, leftBehindCount[t]).Scan(&c.Rows); err != nil {
				return CarrySource{}, fmt.Errorf("ops: count %s: %w", t, err)
			}
		}
		src.Left = append(src.Left, c)
	}
	return src, nil
}

// checkOtherBodies refuses a file holding template bodies under the name its version
// does not use. An empty table under that name is harmless and is ignored, since a build
// of the other version that opened the file and refused it creates its own empty table
// before it reads the version. Rows there were written by a build of neither version,
// and the carry cannot say which of the two tables is the store's.
func checkOtherBodies(ctx context.Context, tx *sql.Tx, version int, tables map[string]bool) error {
	for v, other := range templateBodies {
		if v == version || !tables[other] {
			continue
		}
		var n int
		if err := tx.QueryRowContext(ctx, otherBodiesCount[other]).Scan(&n); err != nil {
			return fmt.Errorf("ops: count %s: %w", other, err)
		}
		if n > 0 {
			return fmt.Errorf("%w: a version %d store holds its template bodies in %s, and this file also holds %d rows in %s, "+
				"which no released build of version %d writes", ErrCarrySource, version, templateBodies[version], n, other, version)
		}
	}
	return nil
}

// otherBodiesCount counts a template body table by its name. The names are constants.
var otherBodiesCount = map[string]string{
	"inline_projection": `SELECT COUNT(*) FROM inline_projection`,
	"template_body":     `SELECT COUNT(*) FROM template_body`,
}

// tableNames is the set of tables the file holds.
func tableNames(ctx context.Context, tx *sql.Tx) (map[string]bool, error) {
	rows, err := tx.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return nil, fmt.Errorf("ops: list tables: %w", err)
	}
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("ops: list tables: %w", err)
		}
		out[name] = true
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("ops: list tables: %w", err)
	}
	return out, nil
}

// readCarryMeta reads the file's meta and refuses a key the carry does not know.
func readCarryMeta(ctx context.Context, tx *sql.Tx) (ledgerID, bucketWidth string, err error) {
	rows, err := tx.QueryContext(ctx, `SELECT key, value FROM meta ORDER BY key`)
	if err != nil {
		return "", "", fmt.Errorf("ops: read meta: %w", err)
	}
	var unknown []string
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			_ = rows.Close()
			return "", "", fmt.Errorf("ops: read meta: %w", err)
		}
		switch {
		case !carryMeta[k]:
			unknown = append(unknown, k)
		case k == "ledger_id":
			ledgerID = v
		case k == "bucket_width":
			bucketWidth = v
		}
	}
	if err := closeRows(rows); err != nil {
		return "", "", fmt.Errorf("ops: read meta: %w", err)
	}
	if len(unknown) > 0 {
		return "", "", fmt.Errorf("%w: its meta holds %s, which the carry does not know where to put",
			ErrCarrySource, strings.Join(unknown, ", "))
	}
	return ledgerID, bucketWidth, nil
}

// check refuses a source whose content rows do not hash to their keys, naming every
// one, so a damaged file is found by the read and not halfway through the write.
func (s CarrySource) check() error {
	var bad []string
	for _, b := range s.TemplateBodies {
		if checkDigest(b.Hash) != nil || Digest(b.Body) != b.Hash {
			bad = append(bad, KindTemplateBody+" "+b.Hash)
		}
	}
	for _, c := range s.Content {
		if checkDigest(c.Hash) != nil || Digest(c.Body) != c.Hash {
			bad = append(bad, c.Kind+" "+c.Hash)
		}
	}
	if len(bad) > 0 {
		return fmt.Errorf("%w: %d rows do not hash to their keys: %s", ErrCorrupt, len(bad), strings.Join(bad, ", "))
	}
	return nil
}

// carryTarget is every table a carry writes, with how it counts what is there already.
var carryTarget = []struct{ table, count string }{
	{"registered_definition", `SELECT COUNT(*) FROM registered_definition`},
	{"registered_projection", `SELECT COUNT(*) FROM registered_projection`},
	{"template_body", `SELECT COUNT(*) FROM template_body`},
	{"admission_hold", `SELECT COUNT(*) FROM admission_hold`},
	{"receipt", `SELECT COUNT(*) FROM receipt`},
	{"served_value", `SELECT COUNT(*) FROM served_value`},
	{"saved_selection", `SELECT COUNT(*) FROM saved_selection`},
	{"saved_selection_code", `SELECT COUNT(*) FROM saved_selection_code`},
	{"definition", `SELECT COUNT(*) FROM definition`},
}

// CarrySchema writes everything r authored into this store, which must be fresh. That is
// the registered Definitions and projections with their registration moments, the template
// bodies, the admission holds, the receipts, served values, saved selections and their
// codes, and the provisioned Definitions, a type the catalog has dropped included. It is
// one transaction, all of it or none. A store that already holds any of those rows is
// refused with ErrCarryTarget before anything is written, so a second carry into the
// same store fails rather than half-merging two stores; a daemon that has started on
// the store has provisioned it, so a carry runs before the first start.
func (d *DB) CarrySchema(ctx context.Context, r Registered) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ops: carry: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	var held []string
	for _, t := range carryTarget {
		var n int
		if err := tx.QueryRowContext(ctx, t.count).Scan(&n); err != nil {
			return fmt.Errorf("ops: carry: count %s: %w", t.table, err)
		}
		if n > 0 {
			held = append(held, fmt.Sprintf("%d %s", n, t.table))
		}
	}
	if len(held) > 0 {
		sort.Strings(held)
		return fmt.Errorf("%w: it holds %s", ErrCarryTarget, strings.Join(held, ", "))
	}
	if err := carryRows(ctx, tx, r); err != nil {
		return err
	}
	for _, x := range r.Provisioned {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO definition (type, version, body) VALUES (?, ?, ?)`, x.Type, x.Version, x.Body); err != nil {
			return fmt.Errorf("ops: carry provisioned definition %s v%d: %w", x.Type, x.Version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ops: carry: %w", busy(err))
	}
	return nil
}
