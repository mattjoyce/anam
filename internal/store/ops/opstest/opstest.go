// Package opstest is for tests of code that sits above the operational store, and it
// lives beside the store because only the store may name SQLite.
//
// A test of what a reader does with a damaged row needs one, and a test outside the
// store has no way to make one: every write the store offers files a row under the hash
// of its bytes. Corrupt is that way. It is not part of the store's surface and nothing
// outside a test imports it.
package opstest

import (
	"context"
	"fmt"

	"github.com/mattjoyce/anam/internal/store/ops"
)

// replace maps a kind of stored content to the statement that files a row in its table,
// as the schema names them.
var replace = map[string]string{
	ops.KindCheckpoint:    `INSERT OR REPLACE INTO checkpoint_value (hash, body) VALUES (?, ?)`,
	ops.KindReceipt:       `INSERT OR REPLACE INTO receipt (hash, body) VALUES (?, ?)`,
	ops.KindServed:        `INSERT OR REPLACE INTO served_value (hash, body) VALUES (?, ?)`,
	ops.KindSelection:     `INSERT OR REPLACE INTO saved_selection (hash, body) VALUES (?, ?)`,
	ops.KindSelectionCode: `INSERT OR REPLACE INTO saved_selection_code (hash, body) VALUES (?, ?)`,
}

// Corrupt files body under digest in the table kind names, replacing the row there, so
// the row holds bytes that do not hash to its key. It writes the row the way bit rot or
// a hand edit of the file would leave it. No trigger is dropped or disabled to do it:
// the store's triggers refuse an update and a delete, and a replacing insert is neither.
func Corrupt(ctx context.Context, db *ops.DB, kind, digest string, body []byte) error {
	query, ok := replace[kind]
	if !ok {
		return fmt.Errorf("opstest: %q is not a kind of stored content", kind)
	}
	if _, err := db.SQL().ExecContext(ctx, query, digest, body); err != nil {
		return fmt.Errorf("opstest: corrupt %s %s: %w", kind, digest, err)
	}
	return nil
}

// LoseCheckpointValue removes a checkpoint value although the log still names it, as a
// cache that lost an entry would.
func LoseCheckpointValue(ctx context.Context, db *ops.DB, digest string) error {
	if _, err := db.SQL().ExecContext(ctx, `DELETE FROM checkpoint_value WHERE hash = ?`, digest); err != nil {
		return fmt.Errorf("opstest: lose %s: %w", digest, err)
	}
	return nil
}

// LoseCheckpointRow removes one checkpoint row, and its recorded size, from the log, as a
// cache that lost an entry would. The mint runs that read it are left as they were.
func LoseCheckpointRow(ctx context.Context, db *ops.DB, spanID string, v int) error {
	for _, q := range []string{
		`DELETE FROM checkpoint_size WHERE span_id = ? AND v = ?`,
		`DELETE FROM checkpoint WHERE span_id = ? AND v = ?`,
	} {
		if _, err := db.SQL().ExecContext(ctx, q, spanID, v); err != nil {
			return fmt.Errorf("opstest: lose %s v%d: %w", spanID, v, err)
		}
	}
	return nil
}
