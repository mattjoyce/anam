package ops

import (
	"context"
	"database/sql"
	"fmt"
)

// RegisteredDefinition is a registered Definition with the moment it was registered
// (clock layout), as the table holds it.
type RegisteredDefinition struct {
	DefinitionRow
	RegisteredAt string
}

// RegisteredProjection is a registered projection template with the moment it was
// registered (clock layout), as the table holds it.
type RegisteredProjection struct {
	ProjectionRow
	RegisteredAt string
}

// InlineProjection is the body of an inline projection with the digest that addresses it.
type InlineProjection struct {
	Hash string
	Body []byte
}

// Registered is what one operational store holds that its ledger cannot give back
// (card 200): every Definition and projection version registered, immutable, and the
// Definition bodies the store was provisioned with. A migration reads it from the old
// store in one transaction and writes the registered rows into the new one, so every
// Definition version a carried facet was admitted under is held there, and the
// registered projections are served as they were.
type Registered struct {
	Definitions []RegisteredDefinition
	Projections []RegisteredProjection

	// Inline are the bodies of the projections queries carried in their requests
	// (card 211), each addressed by the hash its receipts record. They are authored
	// and immutable like the registered rows, and a receipt that names one cannot be
	// replayed without it, so a migration copies them too.
	Inline []InlineProjection

	// Provisioned is the store's copy of the Definitions the binary carries, bundled
	// and history. It is read so a migration can resolve an older version; it is not
	// written, because the daemon provisions the new store's own at start.
	Provisioned []DefinitionRow
}

// ReadRegistered reads the registered rows and the provisioned Definitions in one read
// transaction, so they are one consistent view of the store.
func (d *DB) ReadRegistered(ctx context.Context) (Registered, error) {
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Registered{}, fmt.Errorf("ops: read registered: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // a read transaction: nothing to commit

	var out Registered
	rows, err := tx.QueryContext(ctx,
		`SELECT type, version, body, registered_at FROM registered_definition ORDER BY type, version`)
	if err != nil {
		return Registered{}, fmt.Errorf("ops: read registered definitions: %w", err)
	}
	for rows.Next() {
		var r RegisteredDefinition
		if err := rows.Scan(&r.Type, &r.Version, &r.Body, &r.RegisteredAt); err != nil {
			_ = rows.Close()
			return Registered{}, fmt.Errorf("ops: scan registered definition: %w", err)
		}
		out.Definitions = append(out.Definitions, r)
	}
	if err := closeRows(rows); err != nil {
		return Registered{}, fmt.Errorf("ops: read registered definitions: %w", err)
	}

	rows, err = tx.QueryContext(ctx,
		`SELECT name, version, body, registered_at FROM registered_projection ORDER BY name, version`)
	if err != nil {
		return Registered{}, fmt.Errorf("ops: read registered projections: %w", err)
	}
	for rows.Next() {
		var r RegisteredProjection
		if err := rows.Scan(&r.Name, &r.Version, &r.Body, &r.RegisteredAt); err != nil {
			_ = rows.Close()
			return Registered{}, fmt.Errorf("ops: scan registered projection: %w", err)
		}
		out.Projections = append(out.Projections, r)
	}
	if err := closeRows(rows); err != nil {
		return Registered{}, fmt.Errorf("ops: read registered projections: %w", err)
	}

	rows, err = tx.QueryContext(ctx, `SELECT hash, body FROM inline_projection ORDER BY hash`)
	if err != nil {
		return Registered{}, fmt.Errorf("ops: read inline projections: %w", err)
	}
	for rows.Next() {
		var r InlineProjection
		if err := rows.Scan(&r.Hash, &r.Body); err != nil {
			_ = rows.Close()
			return Registered{}, fmt.Errorf("ops: scan inline projection: %w", err)
		}
		out.Inline = append(out.Inline, r)
	}
	if err := closeRows(rows); err != nil {
		return Registered{}, fmt.Errorf("ops: read inline projections: %w", err)
	}

	rows, err = tx.QueryContext(ctx, `SELECT type, version, body FROM definition ORDER BY type, version`)
	if err != nil {
		return Registered{}, fmt.Errorf("ops: read provisioned definitions: %w", err)
	}
	for rows.Next() {
		var r DefinitionRow
		if err := rows.Scan(&r.Type, &r.Version, &r.Body); err != nil {
			_ = rows.Close()
			return Registered{}, fmt.Errorf("ops: scan provisioned definition: %w", err)
		}
		out.Provisioned = append(out.Provisioned, r)
	}
	if err := closeRows(rows); err != nil {
		return Registered{}, fmt.Errorf("ops: read provisioned definitions: %w", err)
	}
	return out, nil
}

// closeRows ends a result set and reports why it stopped.
func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

// CarryRegistered writes the registered Definitions and projections and the inline
// projection bodies of r, each at its
// registration moment, in one transaction: all of them or none. The tables are
// immutable, so it fits a store with none of these versions, which is what a fresh
// store is; a version already there fails the call and leaves the store as it was.
func (d *DB) CarryRegistered(ctx context.Context, r Registered) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ops: carry registered: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit
	for _, x := range r.Definitions {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO registered_definition (type, version, body, registered_at) VALUES (?, ?, ?, ?)`,
			x.Type, x.Version, x.Body, x.RegisteredAt); err != nil {
			return fmt.Errorf("ops: carry definition %s v%d: %w", x.Type, x.Version, err)
		}
	}
	for _, x := range r.Projections {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO registered_projection (name, version, body, registered_at) VALUES (?, ?, ?, ?)`,
			x.Name, x.Version, x.Body, x.RegisteredAt); err != nil {
			return fmt.Errorf("ops: carry projection %s v%d: %w", x.Name, x.Version, err)
		}
	}
	for _, x := range r.Inline {
		if err := checkDigest(x.Hash); err != nil {
			return fmt.Errorf("ops: carry inline projection: %w", err)
		}
		if got := Digest(x.Body); got != x.Hash {
			return fmt.Errorf("%w: inline projection %s hashes to %s", ErrCorrupt, x.Hash, got)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO inline_projection (hash, body) VALUES (?, ?)`, x.Hash, x.Body); err != nil {
			return fmt.Errorf("ops: carry inline projection %s: %w", x.Hash, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ops: carry registered: %w", err)
	}
	return nil
}
