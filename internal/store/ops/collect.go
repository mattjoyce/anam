package ops

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
)

// Collected is what one CollectCheckpoints removed.
type Collected struct {
	// Runs, Checkpoints and Values count the mint runs, checkpoint rows and checkpoint
	// values removed; Bytes is the size of the values.
	Runs, Checkpoints, Values int
	Bytes                     int64
}

// None reports whether the collection removed nothing.
func (c Collected) None() bool { return c == Collected{} }

// CollectCheckpoints removes the checkpoints nothing reads any more, after a mint
// (ARCHITECTURE.md §7: a checkpoint is only ever a replay floor, and may go once a
// later one subsumes it and no receipt pins it).
//
// A read at W starts from the latest mint run at or below W and takes, for each span,
// the latest checkpoint at or below that run (CheckpointsAt). So what is still read is
// the newest run, which every mint starts from, and each run a receipt pins: the latest
// run at or below the receipt's watermark, so that verifying it still starts from a
// floor. The checkpoints those runs take are kept, and so is each span's head, whatever
// run reads it. Every other run, checkpoint row and size is removed, then the values no
// row names, and the seal is written again at the rows that remain.
//
// A read at a watermark below the oldest run kept finds no run to start from and folds
// the ledger in full, which is the same value, only slower. That is the whole cost.
//
// It collects only a sealed log (LogSealed). An unsealed one holds rows from a mint that
// crashed, and the next full mint settles them; removing around them would seal them in
// as though a run had accounted for them. The call is one transaction, so a reader sees
// the log before it or after it. Run again, it removes nothing.
func (d *DB) CollectCheckpoints(ctx context.Context) (out Collected, err error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return Collected{}, fmt.Errorf("ops: collect checkpoints: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback() // the error being returned is the one that matters
		}
	}()
	// SQLite takes its write lock at the first write, not at BEGIN, and a transaction
	// that reads first and writes later is refused if another writer got in between.
	// This statement removes nothing; it makes the transaction a writer from the start.
	if _, err = tx.ExecContext(ctx, `DELETE FROM mint_seal WHERE checkpoints < 0`); err != nil {
		return Collected{}, fmt.Errorf("ops: collect checkpoints: %w", err)
	}

	var sealed bool
	if err = tx.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT MAX(checkpoints) FROM mint_seal), -1) = (SELECT COUNT(*) FROM checkpoint)`,
	).Scan(&sealed); err != nil {
		return Collected{}, fmt.Errorf("ops: collect checkpoints: read mint seal: %w", err)
	}
	runs, err := allRuns(ctx, tx)
	if err != nil {
		return Collected{}, err
	}
	if !sealed || len(runs) == 0 {
		return Collected{}, tx.Commit()
	}
	pinned, err := pinnedRuns(ctx, tx)
	if err != nil {
		return Collected{}, err
	}
	rows, err := logRows(ctx, tx)
	if err != nil {
		return Collected{}, err
	}

	keepRuns := map[int64]bool{runs[len(runs)-1]: true}
	for _, r := range pinned {
		keepRuns[r] = true
	}
	keep := keptRows(rows, keepRuns)

	if out, err = removeUnread(ctx, tx, runs, keepRuns, rows, keep); err != nil {
		return Collected{}, err
	}
	if out.Checkpoints > 0 {
		// The log holds fewer rows than the seal says, so seal it at what it holds, at
		// the newest run: the one every row left belongs to, or to an earlier run kept.
		if _, err = tx.ExecContext(ctx, `DELETE FROM mint_seal`); err != nil {
			return Collected{}, fmt.Errorf("ops: collect checkpoints: reseal: %w", err)
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO mint_seal (checkpoints, w) SELECT COUNT(*), (SELECT MAX(w) FROM mint_run) FROM checkpoint`); err != nil {
			return Collected{}, fmt.Errorf("ops: collect checkpoints: reseal: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return Collected{}, fmt.Errorf("ops: collect checkpoints: %w", err)
	}
	return out, nil
}

// logRow is a checkpoint row as collection reads it.
type logRow struct {
	spanID  string
	v       int
	coversW int64
}

func allRuns(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `SELECT w FROM mint_run ORDER BY w`)
	if err != nil {
		return nil, fmt.Errorf("ops: collect checkpoints: read mint runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var w int64
		if err := rows.Scan(&w); err != nil {
			return nil, fmt.Errorf("ops: collect checkpoints: scan mint run: %w", err)
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: collect checkpoints: read mint runs: %w", err)
	}
	return out, nil
}

// pinnedRuns is the run each receipt's watermark reads from: the latest at or below it.
// The watermark is read out of the receipt's own bytes, so there is no second copy of
// it to drift from the receipt. A receipt that names none, or below every run, pins
// nothing.
func pinnedRuns(ctx context.Context, tx *sql.Tx) ([]int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT DISTINCT (SELECT MAX(m.w) FROM mint_run m
		                 WHERE m.w <= json_extract(CAST(r.body AS TEXT), '$.watermark'))
		FROM receipt r WHERE json_valid(CAST(r.body AS TEXT))`)
	if err != nil {
		return nil, fmt.Errorf("ops: collect checkpoints: read receipt pins: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var w sql.NullInt64
		if err := rows.Scan(&w); err != nil {
			return nil, fmt.Errorf("ops: collect checkpoints: scan receipt pin: %w", err)
		}
		if w.Valid {
			out = append(out, w.Int64)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: collect checkpoints: read receipt pins: %w", err)
	}
	return out, nil
}

func logRows(ctx context.Context, tx *sql.Tx) ([]logRow, error) {
	rows, err := tx.QueryContext(ctx, `SELECT span_id, v, covers_w FROM checkpoint ORDER BY span_id, v`)
	if err != nil {
		return nil, fmt.Errorf("ops: collect checkpoints: read the log: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []logRow
	for rows.Next() {
		var r logRow
		if err := rows.Scan(&r.spanID, &r.v, &r.coversW); err != nil {
			return nil, fmt.Errorf("ops: collect checkpoints: scan the log: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: collect checkpoints: read the log: %w", err)
	}
	return out, nil
}

// keptRows is the rows a read from any of runs takes, and each span's head. A run takes,
// for each span, the highest version whose covers_w is at or below it, which is
// CheckpointsAt's rule.
func keptRows(rows []logRow, runs map[int64]bool) map[SpanVersion]bool {
	keep := map[SpanVersion]bool{}
	bySpan := map[string][]logRow{}
	var spans []string
	for _, r := range rows {
		if _, seen := bySpan[r.spanID]; !seen {
			spans = append(spans, r.spanID)
		}
		bySpan[r.spanID] = append(bySpan[r.spanID], r)
	}
	sort.Strings(spans)
	for _, id := range spans {
		versions := bySpan[id] // in v order
		last := versions[len(versions)-1]
		keep[SpanVersion{SpanID: id, V: last.v}] = true
		for run := range runs {
			var pick *logRow
			for i := range versions {
				if versions[i].coversW <= run {
					pick = &versions[i] // the highest v at or below run, as the list is in v order
				}
			}
			if pick != nil {
				keep[SpanVersion{SpanID: id, V: pick.v}] = true
			}
		}
	}
	return keep
}

// removeUnread deletes the runs, rows and sizes outside keep, then the values no row
// names, and counts them.
func removeUnread(ctx context.Context, tx *sql.Tx, runs []int64, keepRuns map[int64]bool, rows []logRow,
	keep map[SpanVersion]bool,
) (Collected, error) {
	var out Collected
	for _, w := range runs {
		if keepRuns[w] {
			continue
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM mint_run WHERE w = ?`, w); err != nil {
			return Collected{}, fmt.Errorf("ops: collect checkpoints: remove run %d: %w", w, err)
		}
		out.Runs++
	}
	for _, r := range rows {
		if keep[SpanVersion{SpanID: r.spanID, V: r.v}] {
			continue
		}
		for _, q := range []string{
			`DELETE FROM checkpoint_size WHERE span_id = ? AND v = ?`,
			`DELETE FROM checkpoint WHERE span_id = ? AND v = ?`,
		} {
			if _, err := tx.ExecContext(ctx, q, r.spanID, r.v); err != nil {
				return Collected{}, fmt.Errorf("ops: collect checkpoints: remove %s v%d: %w", r.spanID, r.v, err)
			}
		}
		out.Checkpoints++
	}
	// Values nothing names go whether or not a row went with them: a value put by a
	// move that no log row wants is as unread as one a collected row named.
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(length(body)), 0) FROM checkpoint_value
		WHERE hash NOT IN (SELECT value_hash FROM checkpoint)`).Scan(&out.Values, &out.Bytes); err != nil {
		return Collected{}, fmt.Errorf("ops: collect checkpoints: measure unread values: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM checkpoint_value WHERE hash NOT IN (SELECT value_hash FROM checkpoint)`); err != nil {
		return Collected{}, fmt.Errorf("ops: collect checkpoints: remove unread values: %w", err)
	}
	return out, nil
}
