package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// The checkpoint log's typed surface (§7, §8). The mint (internal/mint) decides WHAT a
// row says; this package only persists and reads it. The table is append-only by
// convention — every mint of every span is a new row, and the head of a span is the
// span_head view over them — so a checkpoint, like a fact, is never overwritten.
//
// Tier: derived. A checkpoint is a value of the fold at a watermark, so dropping the
// table costs a re-mint, never a fact. The files it points at live in the artifact
// tier, which is authored and backed up (artifact package doc).

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

// AppendMintRun records that a mint at w checked every span, and seals the log at the
// rows it holds now (LogSealed). Recording the same w twice adds no run: a second run
// at an unchanged head checked the same thing, though it may have minted, and its seal
// says so.
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
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO mint_run (w, ran_at) VALUES (?, ?) ON CONFLICT (w) DO NOTHING`, w, ranAt); err != nil {
		return fmt.Errorf("ops: append mint run at %d: %w", w, err)
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO mint_seal (checkpoints, w) SELECT COUNT(*), ? FROM checkpoint WHERE true
		 ON CONFLICT (checkpoints) DO NOTHING`, w); err != nil {
		return fmt.Errorf("ops: seal mint run at %d: %w", w, err)
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("ops: append mint run at %d: %w", w, err)
	}
	return nil
}

// LogSealed reports whether every checkpoint in the log belongs to a recorded run: the
// log holds as many rows as it did when the newest run was recorded. False means a
// mint since then crashed after writing, or no run has been sealed.
func (d *DB) LogSealed(ctx context.Context) (bool, error) {
	var sealed bool
	err := d.db.QueryRowContext(ctx, `
		SELECT COALESCE((SELECT MAX(checkpoints) FROM mint_seal), -1) = (SELECT COUNT(*) FROM checkpoint)`,
	).Scan(&sealed)
	if err != nil {
		return false, fmt.Errorf("ops: read mint seal: %w", err)
	}
	return sealed, nil
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

// CheckpointsAt returns each span's latest checkpoint whose covers_w is at or below w:
// the span's value as a mint at w found it, when w is a mint run.
func (d *DB) CheckpointsAt(ctx context.Context, w int64) ([]CheckpointRow, error) {
	rows, err := d.db.QueryContext(ctx, `
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
