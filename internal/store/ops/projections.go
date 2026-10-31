package ops

import (
	"context"
	"fmt"
)

// ProjectionRow is one registered projection template: the manifest as the operator
// sent it, by name and version.
type ProjectionRow struct {
	Name    string
	Version int
	Body    string
}

// RegisteredProjections returns every registered template, in name then version
// order.
func (d *DB) RegisteredProjections(ctx context.Context) ([]ProjectionRow, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT name, version, body FROM registered_projection ORDER BY name, version`)
	if err != nil {
		return nil, fmt.Errorf("ops: read registered projections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []ProjectionRow
	for rows.Next() {
		var r ProjectionRow
		if err := rows.Scan(&r.Name, &r.Version, &r.Body); err != nil {
			return nil, fmt.Errorf("ops: scan registered projection: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: read registered projections: %w", err)
	}
	return out, nil
}

// RegisterProjection stores a newly registered template, at registeredAt (clock
// layout). A row already at its name and version is refused: whether the same body
// may be registered twice is the caller's question, answered before this.
func (d *DB) RegisterProjection(ctx context.Context, r ProjectionRow, registeredAt string) error {
	if _, err := d.db.ExecContext(ctx,
		`INSERT INTO registered_projection (name, version, body, registered_at) VALUES (?, ?, ?, ?)`,
		r.Name, r.Version, r.Body, registeredAt); err != nil {
		return fmt.Errorf("ops: register projection %s v%d: %w", r.Name, r.Version, err)
	}
	return nil
}
