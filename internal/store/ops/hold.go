package ops

import (
	"context"
	"fmt"
	"strings"
)

// HoldAll is the source key of a global hold: it holds every source but the ones the
// daemon never holds.
const HoldAll = "*"

// Hold is an admission hold: the source it names, and the moment it was set.
type Hold struct {
	Source string
	HeldAt string
}

// SetHold holds a source. Holding a source that is held already changes nothing: the
// hold keeps the moment it was first set, so a second hold is not a new fact.
func (d *DB) SetHold(ctx context.Context, source, heldAt string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("ops: hold source is empty")
	}
	if _, err := d.db.ExecContext(ctx,
		`INSERT INTO admission_hold (source, held_at) VALUES (?, ?) ON CONFLICT(source) DO NOTHING`,
		source, heldAt); err != nil {
		return fmt.Errorf("ops: set hold %s: %w", source, err)
	}
	return nil
}

// ReleaseHold releases one hold, or every hold when the source is empty or the global
// key. Releasing a source that is not held changes nothing.
func (d *DB) ReleaseHold(ctx context.Context, source string) error {
	source = strings.TrimSpace(source)
	if source == "" || source == HoldAll {
		if _, err := d.db.ExecContext(ctx, `DELETE FROM admission_hold`); err != nil {
			return fmt.Errorf("ops: release holds: %w", err)
		}
		return nil
	}
	if _, err := d.db.ExecContext(ctx, `DELETE FROM admission_hold WHERE source = ?`, source); err != nil {
		return fmt.Errorf("ops: release hold %s: %w", source, err)
	}
	return nil
}

// Holds is every hold the store holds, by source.
func (d *DB) Holds(ctx context.Context) ([]Hold, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT source, held_at FROM admission_hold ORDER BY source`)
	if err != nil {
		return nil, fmt.Errorf("ops: read holds: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []Hold
	for rows.Next() {
		var h Hold
		if err := rows.Scan(&h.Source, &h.HeldAt); err != nil {
			return nil, fmt.Errorf("ops: scan hold: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: read holds: %w", err)
	}
	return out, nil
}

// HasHold reports whether the source is held: by its own name, or by the global hold.
func (d *DB) HasHold(ctx context.Context, source string) (bool, error) {
	var n int
	err := d.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM admission_hold WHERE source = ? OR source = ?`, source, HoldAll).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("ops: has hold: %w", err)
	}
	return n > 0, nil
}
