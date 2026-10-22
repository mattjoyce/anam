package ops

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
)

// The checkpoint log's typed surface (§7, §8). The mint (internal/mint) decides WHAT a
// row says; this package only persists and reads it. Every mint of every span is a new
// row, and the head of a span is the span_head view over them.
//
// Tier: derived, and a cache. A checkpoint is a value of the fold at a watermark, so
// dropping the table costs a re-mint, never a fact, and a row or a value that goes
// missing is a miss the reader folds past (RunCheckpoints). The values the rows name
// are in checkpoint_value (content.go).

// CheckpointRow is one checkpoint row: span SpanID at version V holds the value
// ValueHash, folded at watermark CoversW, minted at MintedAt (a clock.Layout string).
type CheckpointRow struct {
	SpanID    string
	V         int
	CoversW   int64
	ValueHash string
	MintedAt  string
}

// SpanHeads returns the latest checkpoint of every span that has one, keyed by
// span id. A span with no row is absent — never minted.
func (d *DB) SpanHeads(ctx context.Context) (map[string]CheckpointRow, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT span_id, v, covers_w, value_hash, minted_at FROM span_head`)
	if err != nil {
		return nil, fmt.Errorf("ops: read span heads: %w", err)
	}
	defer func() { _ = rows.Close() }()

	heads := map[string]CheckpointRow{}
	for rows.Next() {
		var r CheckpointRow
		if err := rows.Scan(&r.SpanID, &r.V, &r.CoversW, &r.ValueHash, &r.MintedAt); err != nil {
			return nil, fmt.Errorf("ops: scan span head: %w", err)
		}
		heads[r.SpanID] = r
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: iterate span heads: %w", err)
	}
	return heads, nil
}

// Checkpoints returns the whole log in (span_id, v) order — every mint of every span.
func (d *DB) Checkpoints(ctx context.Context) ([]CheckpointRow, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT span_id, v, covers_w, value_hash, minted_at FROM checkpoint ORDER BY span_id, v`)
	if err != nil {
		return nil, fmt.Errorf("ops: read checkpoints: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []CheckpointRow
	for rows.Next() {
		var r CheckpointRow
		if err := rows.Scan(&r.SpanID, &r.V, &r.CoversW, &r.ValueHash, &r.MintedAt); err != nil {
			return nil, fmt.Errorf("ops: scan checkpoint: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: iterate checkpoints: %w", err)
	}
	return out, nil
}

// AppendCheckpoint adds one row to the log. The primary key is (span_id, v), so two
// mints racing to the same version collide here rather than one silently winning:
// the loser's error says to re-run, and the re-run finds the span already at head.
func (d *DB) AppendCheckpoint(ctx context.Context, r CheckpointRow) error {
	_, err := d.db.ExecContext(ctx, `
		INSERT INTO checkpoint (span_id, v, covers_w, value_hash, minted_at)
		VALUES (?, ?, ?, ?, ?)`,
		r.SpanID, r.V, r.CoversW, r.ValueHash, r.MintedAt)
	if err != nil {
		return fmt.Errorf("ops: append checkpoint %s v%d: %w", r.SpanID, r.V, err)
	}
	return nil
}

// AppendMintRun records that a mint at w checked every span, with a digest of the
// checkpoints a read from w takes (RunCheckpoints). Recording the same w twice replaces
// the digest: a second run at an unchanged head checked the same thing, though it may
// have minted rows at that w (a lowered budget splits with no new event), and the digest
// says which rows now belong to the run.
func (d *DB) AppendMintRun(ctx context.Context, w int64, ranAt string) (err error) {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ops: append mint run at %d: %w", w, err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback() // the error being returned is the one that matters
		}
	}()
	picks, err := checkpointsAt(ctx, tx, w)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO mint_run (w, ran_at, picks) VALUES (?, ?, ?)
		 ON CONFLICT (w) DO UPDATE SET picks = excluded.picks`, w, ranAt, picksDigest(picks)); err != nil {
		return fmt.Errorf("ops: append mint run at %d: %w", w, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("ops: append mint run at %d: %w", w, err)
	}
	return nil
}

// picksDigest is the digest of a run's checkpoints, in span order: each span, version
// and value hash. A different set, a different version or a different value is a
// different digest.
func picksDigest(rows []CheckpointRow) string {
	h := sha256.New()
	for _, r := range rows {
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00%s\n", r.SpanID, r.V, r.ValueHash) // a hash.Hash never fails a write
	}
	return hex.EncodeToString(h.Sum(nil))
}

// RunCheckpoints returns the checkpoints a read starting from the mint run at run takes,
// read in one transaction with the digest the run recorded. ok is false when the run is
// not recorded or when the log no longer yields the checkpoints the run saw: one of its
// rows has gone, been replaced, or another has joined it. A caller that gets false has
// no floor to start from and folds the ledger in full. This is what keeps a log that
// lost entries from composing a value with a bucket missing or stale in it.
func (d *DB) RunCheckpoints(ctx context.Context, run int64) (rows []CheckpointRow, ok bool, err error) {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, false, fmt.Errorf("ops: read run %d: %w", run, err)
	}
	defer func() { _ = tx.Rollback() }() // a read transaction: nothing to commit
	return runCheckpoints(ctx, tx, run)
}

func runCheckpoints(ctx context.Context, q logReader, run int64) ([]CheckpointRow, bool, error) {
	var recorded string
	switch err := q.QueryRowContext(ctx, `SELECT picks FROM mint_run WHERE w = ?`, run).Scan(&recorded); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, false, nil
	case err != nil:
		return nil, false, fmt.Errorf("ops: read run %d: %w", run, err)
	}
	rows, err := checkpointsAt(ctx, q, run)
	if err != nil {
		return nil, false, err
	}
	if picksDigest(rows) != recorded {
		return nil, false, nil
	}
	return rows, true, nil
}

// LogSealed reports whether the log is as its newest mint run left it: the run's
// checkpoints are all there, and no checkpoint was written after it. False means no
// run was recorded, a row of the run is gone, or a mint since crashed after writing, and
// a mint from the last run would start from rows it cannot vouch for, so it folds in full.
func (d *DB) LogSealed(ctx context.Context) (bool, error) {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return false, fmt.Errorf("ops: read the log's seal: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // a read transaction: nothing to commit
	var newest sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT MAX(w) FROM mint_run`).Scan(&newest); err != nil {
		return false, fmt.Errorf("ops: read mint runs: %w", err)
	}
	if !newest.Valid {
		return false, nil
	}
	if _, ok, err := runCheckpoints(ctx, tx, newest.Int64); err != nil || !ok {
		return false, err
	}
	var later int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM checkpoint WHERE covers_w > ?`, newest.Int64).Scan(&later); err != nil {
		return false, fmt.Errorf("ops: read the log's seal: %w", err)
	}
	return later == 0, nil
}

// LastMintRun is the watermark of the latest mint run at or below w. ok is false
// when there is none.
func (d *DB) LastMintRun(ctx context.Context, w int64) (run int64, ok bool, err error) {
	var got sql.NullInt64
	if err := d.db.QueryRowContext(ctx, `SELECT MAX(w) FROM mint_run WHERE w <= ?`, w).Scan(&got); err != nil {
		return 0, false, fmt.Errorf("ops: read mint runs: %w", err)
	}
	return got.Int64, got.Valid, nil
}

// logReader is what a read of the log needs: a connection or a transaction on one.
type logReader interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// CheckpointsAt returns each span's latest checkpoint whose covers_w is at or below w:
// the span's value as a mint at w found it, when w is a mint run. It cannot say a row
// is missing; RunCheckpoints can.
func (d *DB) CheckpointsAt(ctx context.Context, w int64) ([]CheckpointRow, error) {
	return checkpointsAt(ctx, d.db, w)
}

func checkpointsAt(ctx context.Context, q logReader, w int64) ([]CheckpointRow, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT span_id, v, covers_w, value_hash, minted_at FROM checkpoint c
		WHERE v = (SELECT MAX(v) FROM checkpoint WHERE span_id = c.span_id AND covers_w <= ?)
		ORDER BY span_id`, w)
	if err != nil {
		return nil, fmt.Errorf("ops: read checkpoints at %d: %w", w, err)
	}
	defer func() { _ = rows.Close() }()

	var out []CheckpointRow
	for rows.Next() {
		var r CheckpointRow
		if err := rows.Scan(&r.SpanID, &r.V, &r.CoversW, &r.ValueHash, &r.MintedAt); err != nil {
			return nil, fmt.Errorf("ops: scan checkpoint: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: iterate checkpoints: %w", err)
	}
	return out, nil
}

// LedgerID is the ledger the store's derived tier was built from, or "" before any
// was pinned.
func (d *DB) LedgerID(ctx context.Context) (string, error) {
	var id string
	err := d.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'ledger_id'`).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", nil
	case err != nil:
		return "", fmt.Errorf("ops: read ledger pin: %w", err)
	}
	return id, nil
}

// AppendCheckpointSize records how many facets checkpoint span v holds.
func (d *DB) AppendCheckpointSize(ctx context.Context, span string, v, facets int) error {
	_, err := d.db.ExecContext(ctx,
		`INSERT INTO checkpoint_size (span_id, v, facets) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, span, v, facets)
	if err != nil {
		return fmt.Errorf("ops: append checkpoint size %s v%d: %w", span, v, err)
	}
	return nil
}

// CheckpointSizes returns the recorded facet count of every checkpoint, keyed by
// span id and version.
func (d *DB) CheckpointSizes(ctx context.Context) (map[SpanVersion]int, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT span_id, v, facets FROM checkpoint_size`)
	if err != nil {
		return nil, fmt.Errorf("ops: read checkpoint sizes: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := map[SpanVersion]int{}
	for rows.Next() {
		var k SpanVersion
		var n int
		if err := rows.Scan(&k.SpanID, &k.V, &n); err != nil {
			return nil, fmt.Errorf("ops: scan checkpoint size: %w", err)
		}
		out[k] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: iterate checkpoint sizes: %w", err)
	}
	return out, nil
}

// SpanVersion names one checkpoint.
type SpanVersion struct {
	SpanID string
	V      int
}
