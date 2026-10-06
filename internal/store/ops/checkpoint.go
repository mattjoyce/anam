package ops

import (
	"context"
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
