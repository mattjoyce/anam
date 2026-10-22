package ops

import (
	"context"
	"fmt"
)

// CollectCheckpoints removes, after a mint, the checkpoint rows, sizes and values that
// the newest mint run does not read, in one transaction (ARCHITECTURE.md §7).
//
// Nothing else needs keeping, because a checkpoint is a cache. A read at W starts from
// the latest run at or below W and takes the checkpoints that run saw; if any of them is
// gone it declines, and the caller folds the ledger in full, which is the same value and
// only slower (RunCheckpoints). A receipt records a watermark and not a checkpoint, and
// verifying it re-derives from the ledger either way, so no receipt holds a checkpoint
// in place. Old runs stay in mint_run: one whose checkpoints are still the newest run's
// reads as before, and one whose are gone declines.
//
// What the newest run reads is each span's latest checkpoint at or below its watermark,
// the same rule as CheckpointsAt. A row written after it, by a mint that crashed, is not
// read and goes too; the next mint settles every span against the fold. With no run
// recorded nothing is read, and nothing is removed.
func (d *DB) CollectCheckpoints(ctx context.Context) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ops: collect checkpoints: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, q := range []string{
		// IS NOT, because a span with no row at or below the run has a NULL pick.
		`DELETE FROM checkpoint
		 WHERE EXISTS (SELECT 1 FROM mint_run)
		   AND v IS NOT (SELECT MAX(c.v) FROM checkpoint c
		                 WHERE c.span_id = checkpoint.span_id AND c.covers_w <= (SELECT MAX(w) FROM mint_run))`,
		`DELETE FROM checkpoint_size
		 WHERE NOT EXISTS (SELECT 1 FROM checkpoint c WHERE c.span_id = checkpoint_size.span_id AND c.v = checkpoint_size.v)`,
		`DELETE FROM checkpoint_value WHERE EXISTS (SELECT 1 FROM mint_run)
		   AND hash NOT IN (SELECT value_hash FROM checkpoint)`,
	} {
		if _, err := tx.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("ops: collect checkpoints: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ops: collect checkpoints: %w", err)
	}
	return nil
}
