package ledger

import (
	"context"
	"database/sql"
	_ "embed" // go:embed for the migration files
	"fmt"
	"strconv"
	"strings"

	"github.com/mattjoyce/anam/internal/clock"
)

// A migration carries an older ledger file forward one schema version. It is DDL
// only: it may add to what the file holds and never drops, rewrites or re-keys a
// row, because the rows are the record. DDL does not fire the append-only triggers,
// so a migration runs under them untouched.
//
// Each step runs in one transaction with the ledger_meta key that records it, so a
// crash leaves the file at the version before the step, never between. The record
// is append-only like every other pin: the created-at version stays as written and
// the migrations accrete beside it, which is also the file's schema history.
type migration struct {
	to  int
	ddl string

	// rebuild, when set, says this step cannot carry a file forward: what changed is
	// not the shape of the rows but what the rows mean, and only a new ledger from
	// the sources holds the new meaning. The text is the remedy, shown to the
	// operator.
	rebuild string
}

// migration0002 is the step to schema 2, in migrations/0002.sql. Schema 3 has no file:
// it changed no SQL, and its step is the rebuild text below.
//
//go:embed migrations/0002.sql
var migration0002 string

var migrations = []migration{
	{to: 2, ddl: migration0002},
	{to: 3, rebuild: "schema 3 changed how a facet's id is composed from its source row and the zone " +
		"its fact-time is read in; the rows in a schema 2 file were keyed and placed by the old rules. " +
		"Move this file aside (it stays the record of what Anam knew until now), run `anam init` at " +
		"the same path, and have each importer resend its source whole: every source still holds its rows"},
}

// effectiveVersion derives the file's schema version from its pins: the created-at
// version, raised by every migration recorded since.
func effectiveVersion(pins map[string]string) (int, error) {
	stored, ok := pins[metaSchemaVersion]
	if !ok {
		return 0, fmt.Errorf("no %s pin", metaSchemaVersion)
	}
	v, err := strconv.Atoi(stored)
	if err != nil {
		return 0, fmt.Errorf("%s pin %q is not a number", metaSchemaVersion, stored)
	}
	for key := range pins {
		if !strings.HasPrefix(key, metaMigrationPrefix) {
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(key, metaMigrationPrefix))
		if err != nil {
			return 0, fmt.Errorf("pin %q is not a migration record", key)
		}
		v = max(v, n)
	}
	return v, nil
}

// checkVersion compares a file's effective version against this build's.
func checkVersion(have int) error {
	if have == schemaVersion {
		return nil
	}
	return fmt.Errorf("%w: ledger holds schema %d, this build writes %d. Migrate it (`anam init`) or start a new ledger at another path. Do not delete it, it is the record",
		ErrSchemaMismatch, have, schemaVersion)
}

// migrate applies every step from the file's version up to this build's, one
// transaction per step. A file ahead of the build is refused: the build cannot know
// what a later schema holds.
func (l *Ledger) migrate(ctx context.Context, from int) error {
	if from > schemaVersion {
		return checkVersion(from)
	}
	// A step that cannot be applied refuses the whole path before any step runs, so a
	// refused file is exactly as it was: not half-way to a version it can never reach.
	for _, m := range migrations {
		if m.to > from && m.rebuild != "" {
			return fmt.Errorf("%w: ledger %s holds schema %d and cannot migrate to %d: %s",
				ErrSchemaMismatch, l.path, from, m.to, m.rebuild)
		}
	}
	for _, m := range migrations {
		if m.to <= from {
			continue
		}
		if err := l.migrateOne(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

func (l *Ledger) migrateOne(ctx context.Context, m migration) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return classify("begin migration", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if _, err := tx.ExecContext(ctx, m.ddl); err != nil {
		return classify(fmt.Sprintf("migrate to schema %d", m.to), err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO ledger_meta (key, value) VALUES (?, ?)`,
		metaMigrationPrefix+strconv.Itoa(m.to), clock.Format(clock.Real{}.Now())); err != nil {
		return classify(fmt.Sprintf("record migration to schema %d", m.to), err)
	}
	if err := tx.Commit(); err != nil {
		return classify("commit migration", err)
	}
	return nil
}

// readPins loads ledger_meta whole.
func readPins(ctx context.Context, q interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}) (map[string]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT key, value FROM ledger_meta`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	pins := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, classify("scan pin", err)
		}
		pins[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, classify("read pins", err)
	}
	return pins, nil
}
