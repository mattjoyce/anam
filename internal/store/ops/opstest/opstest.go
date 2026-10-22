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
// store whose values were not carried across would be. The store refuses to delete a
// value a checkpoint row names, so the rows that name it are lifted for the delete and
// put back as they were, in one transaction.
func LoseCheckpointValue(ctx context.Context, db *ops.DB, digest string) error {
	tx, err := db.SQL().BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("opstest: lose %s: %w", digest, err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	rows, err := tx.QueryContext(ctx,
		`SELECT span_id, v, covers_w, value_hash, minted_at FROM checkpoint WHERE value_hash = ?`, digest)
	if err != nil {
		return fmt.Errorf("opstest: lose %s: %w", digest, err)
	}
	var named []ops.CheckpointRow
	for rows.Next() {
		var r ops.CheckpointRow
		if err := rows.Scan(&r.SpanID, &r.V, &r.CoversW, &r.ValueHash, &r.MintedAt); err != nil {
			_ = rows.Close()
			return fmt.Errorf("opstest: lose %s: %w", digest, err)
		}
		named = append(named, r)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("opstest: lose %s: %w", digest, err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("opstest: lose %s: %w", digest, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM checkpoint WHERE value_hash = ?`, digest); err != nil {
		return fmt.Errorf("opstest: lose %s: %w", digest, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM checkpoint_value WHERE hash = ?`, digest); err != nil {
		return fmt.Errorf("opstest: lose %s: %w", digest, err)
	}
	for _, r := range named {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO checkpoint (span_id, v, covers_w, value_hash, minted_at) VALUES (?, ?, ?, ?, ?)`,
			r.SpanID, r.V, r.CoversW, r.ValueHash, r.MintedAt); err != nil {
			return fmt.Errorf("opstest: lose %s: %w", digest, err)
		}
	}
	return tx.Commit()
}
