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

// TemplateBody is a template body with the digest that addresses it.
type TemplateBody struct {
	Hash string
	Body []byte
}

// Content is one row of an authored content table: its kind, the digest that keys it
// and its bytes.
type Content struct {
	Kind string
	Hash string
	Body []byte
}

// carriedContent are the authored content kinds a carry copies besides the template
// bodies, which Registered holds apart: receipts, the values they served, saved
// selections and their codes. Checkpoint values are derived; a mint makes them again.
var carriedContent = []string{KindReceipt, KindServed, KindSelection, KindSelectionCode}

// Registered is what one operational store holds that its ledger cannot give back
// (card 200): every Definition and projection version registered, immutable, and the
// Definition bodies the store was provisioned with. A migration reads it from the old
// store in one transaction and writes the registered rows into the new one, so every
// Definition version a carried facet was admitted under is held there, and the
// registered projections are served as they were.
type Registered struct {
	Definitions []RegisteredDefinition
	Projections []RegisteredProjection

	// TemplateBodies are the template bodies receipts name, each addressed by the
	// hash its receipts record. They are authored and immutable like the registered
	// rows, and a receipt that names one cannot be replayed without it, so a migration
	// copies them too.
	TemplateBodies []TemplateBody
	// Holds are the admission holds the operator has set. They are authored state,
	// like the registered rows: a migration carries them into the new store, so a
	// source held before the move is still held after it.
	Holds []Hold
	// Content are the receipts, served values, saved selections and selection codes
	// (card 224). What Anam showed and what the person saved come back from this
	// store or from nowhere, so a carry copies every row, byte for byte.
	Content []Content

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

	// A file can hold bodies under the old name too: all of them if this build has never
	// opened it, the latest few if an older build filed one after this build copied. A
	// read-only handle cannot copy, so the read takes the union.
	bodies := `SELECT hash, body FROM template_body ORDER BY hash`
	if old, err := hasTable(ctx, tx, "inline_projection"); err != nil {
		return Registered{}, err
	} else if old {
		bodies = `SELECT hash, body FROM template_body UNION SELECT hash, body FROM inline_projection ORDER BY hash`
	}
	rows, err = tx.QueryContext(ctx, bodies)
	if err != nil {
		return Registered{}, fmt.Errorf("ops: read template bodies: %w", err)
	}
	for rows.Next() {
		var r TemplateBody
		if err := rows.Scan(&r.Hash, &r.Body); err != nil {
			_ = rows.Close()
			return Registered{}, fmt.Errorf("ops: scan template body: %w", err)
		}
		out.TemplateBodies = append(out.TemplateBodies, r)
	}
	if err := closeRows(rows); err != nil {
		return Registered{}, fmt.Errorf("ops: read template bodies: %w", err)
	}

	rows, err = tx.QueryContext(ctx, `SELECT source, held_at FROM admission_hold ORDER BY source`)
	if err != nil {
		return Registered{}, fmt.Errorf("ops: read holds: %w", err)
	}
	for rows.Next() {
		var h Hold
		if err := rows.Scan(&h.Source, &h.HeldAt); err != nil {
			_ = rows.Close()
			return Registered{}, fmt.Errorf("ops: scan hold: %w", err)
		}
		out.Holds = append(out.Holds, h)
	}
	if err := closeRows(rows); err != nil {
		return Registered{}, fmt.Errorf("ops: read holds: %w", err)
	}

	for _, kind := range carriedContent {
		if out.Content, err = readContent(ctx, tx, kind, out.Content); err != nil {
			return Registered{}, err
		}
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

// readContent appends every row of kind's table to into.
func readContent(ctx context.Context, tx *sql.Tx, kind string, into []Content) ([]Content, error) {
	t, err := tableOf(kind)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, t.all)
	if err != nil {
		return nil, fmt.Errorf("ops: read %s: %w", kind, err)
	}
	for rows.Next() {
		c := Content{Kind: kind}
		if err := rows.Scan(&c.Hash, &c.Body); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("ops: scan %s: %w", kind, err)
		}
		into = append(into, c)
	}
	if err := closeRows(rows); err != nil {
		return nil, fmt.Errorf("ops: read %s: %w", kind, err)
	}
	return into, nil
}

// closeRows ends a result set and reports why it stopped.
func closeRows(rows *sql.Rows) error {
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

// CarryRegistered writes the registered Definitions and projections, the inline
// projection bodies, the admission holds and the authored content (receipts, served
// values, saved selections and their codes) of r, each at its registration moment, in
// one transaction: all of them or none. Every content row is checked against the
// digest that keys it before it is written. The tables are immutable, so it fits a store
// with none of these versions, which is what a fresh store is; a version already there
// fails the call and leaves the store as it was.
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
	for _, x := range r.TemplateBodies {
		if err := checkDigest(x.Hash); err != nil {
			return fmt.Errorf("ops: carry template body: %w", err)
		}
		if got := Digest(x.Body); got != x.Hash {
			return fmt.Errorf("%w: template body %s hashes to %s", ErrCorrupt, x.Hash, got)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO template_body (hash, body) VALUES (?, ?)`, x.Hash, x.Body); err != nil {
			return fmt.Errorf("ops: carry template body %s: %w", x.Hash, err)
		}
	}
	for _, x := range r.Holds {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admission_hold (source, held_at) VALUES (?, ?) ON CONFLICT(source) DO NOTHING`,
			x.Source, x.HeldAt); err != nil {
			return fmt.Errorf("ops: carry hold %s: %w", x.Source, err)
		}
	}
	for _, x := range r.Content {
		if err := carryContent(ctx, tx, x); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ops: carry registered: %w", err)
	}
	return nil
}

// carryContent writes one authored content row inside tx, refusing bytes that do not
// hash to their key and a row already there.
func carryContent(ctx context.Context, tx *sql.Tx, x Content) error {
	t, err := tableOf(x.Kind)
	if err != nil {
		return fmt.Errorf("ops: carry: %w", err)
	}
	if err := checkDigest(x.Hash); err != nil {
		return fmt.Errorf("ops: carry %s: %w", x.Kind, err)
	}
	if got := Digest(x.Body); got != x.Hash {
		return fmt.Errorf("%w: %s %s hashes to %s", ErrCorrupt, x.Kind, x.Hash, got)
	}
	if _, err := tx.ExecContext(ctx, t.carry, x.Hash, x.Body); err != nil {
		return fmt.Errorf("ops: carry %s %s: %w", x.Kind, x.Hash, err)
	}
	return nil
}
