package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
)

// A snapshot is an importer's statement that a run of batches is everything its
// source holds of some types (card 78). Anam keeps which parts were admitted and the
// facet ids they carried; what is absent is computed when asked, never stored.

// ErrSnapshotMismatch is returned when a part names a snapshot already begun with a
// different source, types or part count: it is not the same run.
var ErrSnapshotMismatch = errors.New("ops: snapshot already begun with a different source, types or parts")

// ErrSnapshotPartChanged is returned when a part number was already recorded with
// other envelopes (card 107). A part is a value: a resend of it with the same
// envelopes adds nothing, and one with other envelopes is another part under a used
// label, not a retry. A run that reads its source again is a new run with a new id.
var ErrSnapshotPartChanged = errors.New("ops: snapshot part already recorded with other envelopes")

// SnapshotRetention is how long snapshots are kept, given by the caller from the
// loaded config (snapshot_ttl, snapshot_keep).
type SnapshotRetention struct {
	// Incomplete is how long an incomplete snapshot is kept before it is dropped.
	Incomplete time.Duration
	// Complete is how many complete snapshots are kept per source: the newest is the
	// one retraction plans from.
	Complete int
}

// SnapshotPart is one admitted batch of a snapshot. Digest is the content identity of
// the batch's envelopes, as the caller computes it; the store holds it beside the part
// number and refuses the number again under another digest.
type SnapshotPart struct {
	ID       string
	Source   string
	Types    []string
	Part     int
	Parts    int
	Digest   string
	FacetIDs []string
}

// Snapshot is what is known of one snapshot.
type Snapshot struct {
	ID            string
	Source        string
	Types         []string
	Parts         int
	PartsAdmitted int
	Carried       int
	StartedAt     time.Time
	// StartedW is the ledger's watermark just after the first part was admitted:
	// nothing outside the run lands between a part's admission and its recording.
	// A facet touched after it was sent either by this run (and is carried) or by an
	// incremental run beside it (card 94), so absence is judged only for facets
	// untouched since the run began.
	StartedW    int64
	CompletedAt time.Time // zero until complete
	// CompletedW is the watermark just after the last part was admitted.
	CompletedW int64
}

// Complete reports whether every part has been admitted.
func (s Snapshot) Complete() bool { return !s.CompletedAt.IsZero() }

// RecordSnapshotPart records an admitted part, in one transaction: incomplete
// snapshots older than keep.Incomplete are dropped first; the snapshot is begun on its first
// part; a part already recorded with the same digest adds nothing, and one recorded
// with another digest is ErrSnapshotPartChanged; a new part and its facet ids are
// added; and the snapshot is marked complete when its last missing part lands. w is
// the ledger's watermark just after this part was admitted; the first part's is kept
// as the run's start. Complete snapshots beyond the newest keep.Complete are dropped.
func (d *DB) RecordSnapshotPart(ctx context.Context, p SnapshotPart, at time.Time, w int64, keep SnapshotRetention) (Snapshot, error) {
	types := typesKey(p.Types)
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("ops: snapshot: %w", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	if err := dropStale(ctx, tx, at, keep.Incomplete); err != nil {
		return Snapshot{}, err
	}
	var source, held string
	var parts int
	err = tx.QueryRowContext(ctx, `SELECT source, types, parts FROM snapshot WHERE id = ?`, p.ID).Scan(&source, &held, &parts)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot (id, source, types, parts, started_at, started_w) VALUES (?, ?, ?, ?, ?, ?)`,
			p.ID, p.Source, types, p.Parts, clock.Format(at), w); err != nil {
			return Snapshot{}, fmt.Errorf("ops: snapshot %s: %w", p.ID, err)
		}
	case err != nil:
		return Snapshot{}, fmt.Errorf("ops: snapshot %s: %w", p.ID, err)
	case source != p.Source || held != types || parts != p.Parts:
		return Snapshot{}, fmt.Errorf("%w: %s", ErrSnapshotMismatch, p.ID)
	}
	var recorded string
	err = tx.QueryRowContext(ctx, `SELECT digest FROM snapshot_part WHERE id = ? AND part = ?`, p.ID, p.Part).Scan(&recorded)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if err := addPart(ctx, tx, p, at, w, keep.Complete); err != nil {
			return Snapshot{}, err
		}
	case err != nil:
		return Snapshot{}, fmt.Errorf("ops: snapshot %s part %d: %w", p.ID, p.Part, err)
	case recorded != p.Digest:
		return Snapshot{}, fmt.Errorf("%w: %s part %d", ErrSnapshotPartChanged, p.ID, p.Part)
	default:
		// An exact retry: the part is already a member, with its facets.
	}
	s, _, err := snapshotOf(ctx, tx, p.ID)
	if err != nil {
		return Snapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Snapshot{}, fmt.Errorf("ops: snapshot %s: %w", p.ID, err)
	}
	return s, nil
}

// addPart records a part not yet held, with its facet ids, and completes the snapshot
// when it was the last missing one.
func addPart(ctx context.Context, tx *sql.Tx, p SnapshotPart, at time.Time, w int64, keepComplete int) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshot_part (id, part, digest) VALUES (?, ?, ?)`, p.ID, p.Part, p.Digest); err != nil {
		return fmt.Errorf("ops: snapshot %s part %d: %w", p.ID, p.Part, err)
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO snapshot_facet (id, facet_id) VALUES (?, ?) ON CONFLICT DO NOTHING`)
	if err != nil {
		return fmt.Errorf("ops: snapshot %s: %w", p.ID, err)
	}
	defer func() { _ = stmt.Close() }()
	for _, f := range p.FacetIDs {
		if _, err := stmt.ExecContext(ctx, p.ID, f); err != nil {
			return fmt.Errorf("ops: snapshot %s facet: %w", p.ID, err)
		}
	}
	var admitted int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM snapshot_part WHERE id = ?`, p.ID).Scan(&admitted); err != nil {
		return fmt.Errorf("ops: snapshot %s: %w", p.ID, err)
	}
	if admitted != p.Parts {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `UPDATE snapshot SET completed_at = ?, completed_w = ? WHERE id = ? AND completed_at IS NULL`,
		clock.Format(at), w, p.ID); err != nil {
		return fmt.Errorf("ops: snapshot %s: %w", p.ID, err)
	}
	return keepNewest(ctx, tx, p.Source, keepComplete)
}

// Snapshot returns one snapshot, or ok false when it is not held.
func (d *DB) Snapshot(ctx context.Context, id string) (Snapshot, bool, error) {
	return snapshotOf(ctx, d.db, id)
}

// SnapshotPartDigest returns the digest a part was recorded with, or ok false when
// the part is not held. It lets a caller refuse a changed part before admitting it;
// RecordSnapshotPart checks again, inside its transaction.
func (d *DB) SnapshotPartDigest(ctx context.Context, id string, part int) (string, bool, error) {
	var digest string
	err := d.db.QueryRowContext(ctx, `SELECT digest FROM snapshot_part WHERE id = ? AND part = ?`, id, part).Scan(&digest)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return "", false, nil
	case err != nil:
		return "", false, fmt.Errorf("ops: snapshot %s part %d: %w", id, part, err)
	}
	return digest, true, nil
}

// NewestCompleteSnapshot returns the source's most recently completed snapshot.
func (d *DB) NewestCompleteSnapshot(ctx context.Context, source string) (Snapshot, bool, error) {
	var id string
	err := d.db.QueryRowContext(ctx, `SELECT id FROM snapshot WHERE source = ? AND completed_at IS NOT NULL
		ORDER BY completed_at DESC, id DESC LIMIT 1`, source).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("ops: newest snapshot of %s: %w", source, err)
	}
	return snapshotOf(ctx, d.db, id)
}

// SnapshotFacets returns the facet ids a snapshot's admitted parts carried.
func (d *DB) SnapshotFacets(ctx context.Context, id string) (map[string]bool, error) {
	rows, err := d.db.QueryContext(ctx, `SELECT facet_id FROM snapshot_facet WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("ops: snapshot %s facets: %w", id, err)
	}
	defer func() { _ = rows.Close() }()
	out := map[string]bool{}
	for rows.Next() {
		var f string
		if err := rows.Scan(&f); err != nil {
			return nil, fmt.Errorf("ops: snapshot %s facets: %w", id, err)
		}
		out[f] = true
	}
	return out, rows.Err()
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func snapshotOf(ctx context.Context, q queryer, id string) (Snapshot, bool, error) {
	var s Snapshot
	var types, started string
	var completed sql.NullString
	var completedW sql.NullInt64
	err := q.QueryRowContext(ctx, `SELECT id, source, types, parts, started_at, started_w, completed_at, completed_w,
		(SELECT COUNT(*) FROM snapshot_part WHERE id = s.id),
		(SELECT COUNT(*) FROM snapshot_facet WHERE id = s.id)
		FROM snapshot AS s WHERE id = ?`, id).
		Scan(&s.ID, &s.Source, &types, &s.Parts, &started, &s.StartedW, &completed, &completedW, &s.PartsAdmitted, &s.Carried)
	if errors.Is(err, sql.ErrNoRows) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("ops: snapshot %s: %w", id, err)
	}
	s.Types = strings.Split(types, ",")
	if s.StartedAt, err = clock.Parse(started); err != nil {
		return Snapshot{}, false, fmt.Errorf("ops: snapshot %s started_at: %w", id, err)
	}
	s.CompletedW = completedW.Int64
	if completed.Valid {
		if s.CompletedAt, err = clock.Parse(completed.String); err != nil {
			return Snapshot{}, false, fmt.Errorf("ops: snapshot %s completed_at: %w", id, err)
		}
	}
	return s, true, nil
}

// dropStale removes incomplete snapshots begun more than ttl before at.
func dropStale(ctx context.Context, tx execer, at time.Time, ttl time.Duration) error {
	cut := clock.Format(at.Add(-ttl))
	return dropWhere(ctx, tx, `completed_at IS NULL AND started_at < ?`, cut)
}

// keepNewest drops a source's complete snapshots beyond the newest keepComplete.
func keepNewest(ctx context.Context, tx execer, source string, keepComplete int) error {
	return dropWhere(ctx, tx, `source = ? AND completed_at IS NOT NULL AND id NOT IN
		(SELECT id FROM snapshot WHERE source = ? AND completed_at IS NOT NULL
		 ORDER BY completed_at DESC, id DESC LIMIT ?)`, source, source, keepComplete)
}

func dropWhere(ctx context.Context, tx execer, where string, args ...any) error {
	for _, q := range []string{
		`DELETE FROM snapshot_facet WHERE id IN (SELECT id FROM snapshot WHERE ` + where + `)`,
		`DELETE FROM snapshot_part WHERE id IN (SELECT id FROM snapshot WHERE ` + where + `)`,
		`DELETE FROM snapshot WHERE ` + where,
	} {
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("ops: drop snapshots: %w", err)
		}
	}
	return nil
}

// typesKey is a type list in one canonical form, so the same set compares equal.
func typesKey(types []string) string {
	t := append([]string(nil), types...)
	sort.Strings(t)
	return strings.Join(t, ",")
}
