package ops

import (
	"context"
	"fmt"
)

// DefinitionRow is one stored Definition: the manifest as loaded, by type and
// version. The store keeps every version it has seen so that a later build can be
// checked against the interpretation the ledger's facets were keyed under.
type DefinitionRow struct {
	Type    string
	Version int
	Body    string
}

// Definitions returns every stored Definition, in type then version order.
func (d *DB) Definitions(ctx context.Context) ([]DefinitionRow, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT type, version, body FROM definition ORDER BY type, version`)
	if err != nil {
		return nil, fmt.Errorf("ops: read definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DefinitionRow
	for rows.Next() {
		var r DefinitionRow
		if err := rows.Scan(&r.Type, &r.Version, &r.Body); err != nil {
			return nil, fmt.Errorf("ops: scan definition: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: read definitions: %w", err)
	}
	return out, nil
}

// PutDefinitions stores the given Definitions, replacing a stored body at the same
// type and version. The provisioned tier is a copy of what the binary carries, so
// overwriting it is a reload, not a loss; the guard that decides whether the reload
// is safe runs before this is called.
func (d *DB) PutDefinitions(ctx context.Context, defs []DefinitionRow) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ops: put definitions: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, r := range defs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO definition (type, version, body) VALUES (?, ?, ?)
			 ON CONFLICT(type, version) DO UPDATE SET body = excluded.body`,
			r.Type, r.Version, r.Body); err != nil {
			return fmt.Errorf("ops: put definition %s v%d: %w", r.Type, r.Version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ops: put definitions: %w", err)
	}
	return nil
}

// RegisteredDefinitions returns every registered Definition, in type then version
// order.
func (d *DB) RegisteredDefinitions(ctx context.Context) ([]DefinitionRow, error) {
	rows, err := d.db.QueryContext(ctx,
		`SELECT type, version, body FROM registered_definition ORDER BY type, version`)
	if err != nil {
		return nil, fmt.Errorf("ops: read registered definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []DefinitionRow
	for rows.Next() {
		var r DefinitionRow
		if err := rows.Scan(&r.Type, &r.Version, &r.Body); err != nil {
			return nil, fmt.Errorf("ops: scan registered definition: %w", err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: read registered definitions: %w", err)
	}
	return out, nil
}

// RegisterDefinition stores a newly registered Definition, at registeredAt (clock
// layout). A row already at its type and version is refused: whether the same body
// may be registered twice is the caller's question, answered before this.
func (d *DB) RegisterDefinition(ctx context.Context, r DefinitionRow, registeredAt string) error {
	if _, err := d.db.ExecContext(ctx,
		`INSERT INTO registered_definition (type, version, body, registered_at) VALUES (?, ?, ?, ?)`,
		r.Type, r.Version, r.Body, registeredAt); err != nil {
		return fmt.Errorf("ops: register definition %s v%d: %w", r.Type, r.Version, err)
	}
	return nil
}
