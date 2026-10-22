package ops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// The content-addressed tables (card 215, card 211). Checkpoint values, receipts, served values
// and saved selections used to be files in an artifact directory, each named by the
// SHA-256 of its own bytes. They are rows now, in the same shape: a hash and the bytes
// it names. Content addressing still does real work. A served value's key is the hash
// its receipt already had to record, so the receipt is the pointer and the integrity
// check at once, and every read re-hashes what it returns.
//
// Which tier a table is in is its schema comment's business. Checkpoint values are
// derived, and the rest are authored.

// The kinds of content a store holds, by the names the files were filed under.
const (
	// KindCheckpoint is a checkpoint's value (derived).
	KindCheckpoint = "checkpoint"
	// KindReceipt is a receipt (authored).
	KindReceipt = "receipt"
	// KindServed is a served value, the bytes a receipt's served hash names (authored).
	KindServed = "served"
	// KindSelection is a saved selection's canonical text (authored).
	KindSelection = "selection"
	// KindSelectionCode is the code of a saved selection that lengthened (authored).
	KindSelectionCode = "selection-code"
	// KindInlineProjection is the body of a projection a query carried in its request,
	// filed so the receipt that names its hash can be replayed (authored).
	KindInlineProjection = "inline-projection"
)

// Kinds lists every kind, in the order a move reports them.
func Kinds() []string {
	return []string{KindCheckpoint, KindReceipt, KindServed, KindSelection, KindSelectionCode, KindInlineProjection}
}

// ErrNotFound is returned for a kind and digest with no row.
var ErrNotFound = errors.New("ops: not found")

// ErrInvalid is returned for a kind or digest that cannot name stored content.
var ErrInvalid = errors.New("ops: invalid name")

// ErrCorrupt is returned when stored bytes do not hash to the digest that keys them.
// Content addressing makes this detectable, so it is detected rather than assumed away.
var ErrCorrupt = errors.New("ops: stored bytes do not match their content address")

// digestPattern matches the canonical "sha256:<64 hex>" form.
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// Digest is the content address of b: "sha256:<hex>", the same spelling the fold uses
// for a canonical value's hash, so the two are directly comparable.
func Digest(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// contentTable is the statements for one content table. The table name is one of the
// constants below, never input: the statements are made once, from the name, by
// replacing a placeholder in a fixed text, so no query is assembled from a value a
// caller gave.
type contentTable struct {
	put, get, size, list string
	// all reads every row and carry inserts one that must not be there yet, for the
	// schema-bump carry (registered.go).
	all, carry string
}

func newContentTable(name string) contentTable {
	q := func(text string) string { return strings.ReplaceAll(text, "{table}", name) }
	return contentTable{
		put:  q(`INSERT INTO {table} (hash, body) VALUES (?, ?) ON CONFLICT (hash) DO NOTHING`),
		get:  q(`SELECT body FROM {table} WHERE hash = ?`),
		size: q(`SELECT length(body) FROM {table} WHERE hash = ?`),
		list: q(`SELECT hash FROM {table} ORDER BY hash`),

		all:   q(`SELECT hash, body FROM {table} ORDER BY hash`),
		carry: q(`INSERT INTO {table} (hash, body) VALUES (?, ?)`),
	}
}

// contentTables maps a kind to its statements.
var contentTables = map[string]contentTable{
	KindCheckpoint:    newContentTable("checkpoint_value"),
	KindReceipt:       newContentTable("receipt"),
	KindServed:        newContentTable("served_value"),
	KindSelection:     newContentTable("saved_selection"),
	KindSelectionCode: newContentTable("saved_selection_code"),

	KindInlineProjection: newContentTable("inline_projection"),
}

func tableOf(kind string) (contentTable, error) {
	t, ok := contentTables[kind]
	if !ok {
		return contentTable{}, fmt.Errorf("%w: %q is not a kind of stored content", ErrInvalid, kind)
	}
	return t, nil
}

func checkDigest(digest string) error {
	if !digestPattern.MatchString(digest) {
		return fmt.Errorf("%w: %q is not a sha256:<64 hex> digest", ErrInvalid, digest)
	}
	return nil
}

// Put files b under kind, addressed by its own digest, and returns that digest.
//
// It is idempotent by construction: identical bytes have an identical key, so a
// re-put finds the row already there and writes nothing. A row that exists holding
// different bytes is a corruption of the table, not an update, and is refused: the
// key could only match if the bytes did.
func (d *DB) Put(ctx context.Context, kind string, b []byte) (string, error) {
	t, err := tableOf(kind)
	if err != nil {
		return "", err
	}
	digest := Digest(b)
	res, err := d.db.ExecContext(ctx, t.put, digest, b)
	if err != nil {
		return "", fmt.Errorf("ops: put %s %s: %w", kind, digest, err)
	}
	if n, _ := res.RowsAffected(); n == 1 {
		return digest, nil
	}
	var existing []byte
	if err := d.db.QueryRowContext(ctx, t.get, digest).Scan(&existing); err != nil {
		return "", fmt.Errorf("ops: put %s %s: read the row already there: %w", kind, digest, err)
	}
	if !bytes.Equal(existing, b) {
		return "", fmt.Errorf("%w: %s %s holds bytes that hash elsewhere", ErrCorrupt, kind, digest)
	}
	return digest, nil
}

// Get returns the bytes stored under kind at digest, verifying on the way out that
// they still hash to the key they are filed under. There is no caller who wanted the
// bytes regardless: every ordinary read of this content is a read of something whose
// integrity is the point.
func (d *DB) Get(ctx context.Context, kind, digest string) ([]byte, error) {
	b, err := d.Raw(ctx, kind, digest)
	if err != nil {
		return nil, err
	}
	if got := Digest(b); got != digest {
		return nil, fmt.Errorf("%w: %s is filed as %s but hashes to %s", ErrCorrupt, kind, digest, got)
	}
	return b, nil
}

// Raw returns the stored bytes without the integrity check, so a caller can report
// precisely how a tampered row differs instead of only that it does. Get is the
// normal read; this exists for the verifier.
func (d *DB) Raw(ctx context.Context, kind, digest string) ([]byte, error) {
	t, err := tableOf(kind)
	if err != nil {
		return nil, err
	}
	if err := checkDigest(digest); err != nil {
		return nil, err
	}
	var b []byte
	switch err := d.db.QueryRowContext(ctx, t.get, digest).Scan(&b); {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("%w: %s %s", ErrNotFound, kind, digest)
	case err != nil:
		return nil, fmt.Errorf("ops: read %s %s: %w", kind, digest, err)
	}
	return b, nil
}

// Size returns how many bytes are stored under kind at digest, from the row alone. It
// does not check the bytes: it answers how big a value is, and a caller that needs the
// bytes uses Get, which checks them.
func (d *DB) Size(ctx context.Context, kind, digest string) (int64, error) {
	t, err := tableOf(kind)
	if err != nil {
		return 0, err
	}
	if err := checkDigest(digest); err != nil {
		return 0, err
	}
	var n int64
	switch err := d.db.QueryRowContext(ctx, t.size, digest).Scan(&n); {
	case errors.Is(err, sql.ErrNoRows):
		return 0, fmt.Errorf("%w: %s %s", ErrNotFound, kind, digest)
	case err != nil:
		return 0, fmt.Errorf("ops: size %s %s: %w", kind, digest, err)
	}
	return n, nil
}

// List returns the digests stored under kind, sorted. Nothing stored yet is a state,
// not a fault.
func (d *DB) List(ctx context.Context, kind string) ([]string, error) {
	t, err := tableOf(kind)
	if err != nil {
		return nil, err
	}
	rows, err := d.db.QueryContext(ctx, t.list)
	if err != nil {
		return nil, fmt.Errorf("ops: list %s: %w", kind, err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var h string
		if err := rows.Scan(&h); err != nil {
			return nil, fmt.Errorf("ops: scan %s: %w", kind, err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: list %s: %w", kind, err)
	}
	return out, nil
}
