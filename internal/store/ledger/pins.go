package ledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/google/uuid"
)

// resolveSubjectPin reads the current name first, falling back to the schema 3
// name. A carried ledger keeps both pins forever; disagreement means damage.
func resolveSubjectPin(pins map[string]string) (string, bool, error) {
	subject, present := pins[metaSubjectID]
	legacy, legacyPresent := pins[metaLegacySubjectID]
	if present && legacyPresent && subject != legacy {
		return "", false, fmt.Errorf("%w: %s pin %q and %s pin %q differ",
			ErrSchemaMismatch, metaSubjectID, subject, metaLegacySubjectID, legacy)
	}
	if present {
		return subject, true, nil
	}
	return legacy, legacyPresent, nil
}

// initIdentity pins a fresh file and checks an existing one. Only step 4 adds the
// new subject key to an existing schema 3 file, in the column rename's transaction.
func (l *Ledger) initIdentity(ctx context.Context, tx *sql.Tx) (map[string]string, error) {
	pins, err := readPins(ctx, tx)
	if err != nil {
		return nil, err
	}
	subject, present, err := resolveSubjectPin(pins)
	if err != nil {
		return nil, err
	}
	if !present {
		subject, err = pin(ctx, tx, metaSubjectID, l.subject)
		if err != nil {
			return nil, err
		}
		pins[metaSubjectID] = subject
	}
	if err := checkSubjectPin(subject, l.subject); err != nil {
		return nil, err
	}
	for _, p := range []struct{ key, want string }{
		{metaSchemaVersion, strconv.Itoa(schemaVersion)},
		{metaLedgerID, uuid.NewString()},
	} {
		pins[p.key], err = pin(ctx, tx, p.key, p.want)
		if err != nil {
			return nil, err
		}
	}
	return pins, nil
}

// pin writes key unless it is already pinned, and returns what is stored. It reads
// before it writes because the insert trigger refuses an existing key outright.
func pin(ctx context.Context, tx *sql.Tx, key, value string) (string, error) {
	var stored string
	err := tx.QueryRowContext(ctx, `SELECT value FROM ledger_meta WHERE key = ?`, key).Scan(&stored)
	switch {
	case err == nil:
		return stored, nil
	case !errors.Is(err, sql.ErrNoRows):
		return "", classify("read pin "+key, err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO ledger_meta (key, value) VALUES (?, ?)`, key, value); err != nil {
		return "", classify("write pin "+key, err)
	}
	return value, nil
}

// checkSubjectPin compares the resolved pin against this configuration.
func checkSubjectPin(stored, want string) error {
	if stored != want {
		return fmt.Errorf("%w: ledger is pinned to %q, config says %q", ErrSubjectMismatch, stored, want)
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
