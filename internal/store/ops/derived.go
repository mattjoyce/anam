package ops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// The derived tier's typed surface. The SQL stays here; catch-up (internal/catchup)
// decides WHAT the rows say, this package only persists and queries them. Both
// index tables come back from replaying the ledger, and ApplyReplay keeps that
// provable by moving the rows and the cursor in one transaction — a crash leaves
// the tier behind, never wrong, and behind is fixable by replay (§8).

// FacetRow is one facet_index row: a logical facet at its current version and
// folded status. Timestamps are clock.Layout strings; BucketID is a span.Bucket id.
type FacetRow struct {
	FacetID     string
	Type        facet.Type
	FactTime    string
	BucketID    string
	Version     int
	Status      facet.Status
	ContentHash string
}

// EventRow is one anam_event_index row.
type EventRow struct {
	Seq             int64
	AnamTime        string
	Kind            facet.AnamEventKind
	FacetID         string
	ArtifactVersion int
}

// LastSeq reports how far the derived tier has replayed the ledger: the seq of the
// last anam-event applied, 0 for a fresh database.
func (d *DB) LastSeq(ctx context.Context) (int64, error) {
	var seq int64
	err := d.db.QueryRowContext(ctx, `SELECT last_seq FROM replay_cursor WHERE id = 1`).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("ops: read replay cursor: %w", err)
	}
	return seq, nil
}

// FacetStates loads the persisted fold states for the given facet ids. A facet
// with no row is simply absent — catch-up treats that as "not yet known", exactly
// the first-event case of fold.Apply. One prepared statement, one lookup per id:
// SQLite is in-process, so N point reads beat assembling an IN clause.
func (d *DB) FacetStates(ctx context.Context, ids []string) (map[string]fold.State, error) {
	states := make(map[string]fold.State, len(ids))
	if len(ids) == 0 {
		return states, nil
	}

	stmt, err := d.db.PrepareContext(ctx,
		`SELECT current_version, status FROM facet_index WHERE facet_id = ?`)
	if err != nil {
		return nil, fmt.Errorf("ops: prepare facet state lookup: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, id := range ids {
		var st fold.State
		err := stmt.QueryRowContext(ctx, id).Scan(&st.Version, &st.Status)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("ops: load facet state %s: %w", id, err)
		}
		states[id] = st
	}
	return states, nil
}

// ApplyReplay lands one catch-up batch: the new anam-events, the changed facets
// at their new states, and the cursor move to uptoSeq — one transaction, so the
// derived tier is never half-applied. The cursor is moved conditionally from
// fromSeq, so two racing catch-ups cannot interleave batches.
func (d *DB) ApplyReplay(ctx context.Context, fromSeq, uptoSeq int64, events []EventRow, facets []FacetRow) error {
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("ops: begin replay tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx, `
		INSERT INTO replay_cursor (id, last_seq) VALUES (1, ?)
		ON CONFLICT (id) DO UPDATE SET last_seq = excluded.last_seq WHERE last_seq = ?`,
		uptoSeq, fromSeq)
	if err != nil {
		return fmt.Errorf("ops: move replay cursor: %w", err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return fmt.Errorf("ops: replay cursor moved since it was read at %d", fromSeq)
	}

	for _, ev := range events {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO anam_event_index (seq, anam_time, kind, facet_id, artifact_version)
			VALUES (?, ?, ?, ?, ?)`,
			ev.Seq, ev.AnamTime, string(ev.Kind), ev.FacetID, ev.ArtifactVersion); err != nil {
			return fmt.Errorf("ops: index anam-event %d: %w", ev.Seq, err)
		}
	}
	for _, f := range facets {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO facet_index (facet_id, type, fact_time, bucket_id, current_version, status, content_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (facet_id) DO UPDATE SET
			  type = excluded.type, fact_time = excluded.fact_time,
			  bucket_id = excluded.bucket_id, current_version = excluded.current_version,
			  status = excluded.status, content_hash = excluded.content_hash`,
			f.FacetID, string(f.Type), f.FactTime, f.BucketID, f.Version, string(f.Status), f.ContentHash); err != nil {
			return fmt.Errorf("ops: index facet %s: %w", f.FacetID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("ops: commit replay tx: %w", err)
	}
	return nil
}

// ChangesSince reports the anam-events in (since, until] by anam-time — what
// changed between two points (§9). Bounds are clock.Layout strings; pass "" as
// until for "to the end". Results come back in (anam_time, seq) order.
//
// Anam-time is not unique, and deliberately so: a batch commit is one change in Anam's
// knowledge, so all its events share one instant. Up to MaxBatch of them can therefore
// sit on a single anam_time, which is why the ordering is (anam_time, seq) — the tie is
// broken by the clock that is total.
//
// That also makes the absence of a LIMIT here load-bearing rather than incidental. The
// bounds are a half-open window on a non-unique column, so a caller paging on the last
// anam_time it saw would step over the rest of that instant's batch. Returning the
// whole window is what keeps the cursor sound; adding a row limit would mean carrying
// seq in the cursor too.

func (d *DB) ChangesSince(ctx context.Context, since, until string) ([]EventRow, error) {
	q := `SELECT seq, anam_time, kind, facet_id, artifact_version
	      FROM anam_event_index WHERE anam_time > ?`
	args := []any{since}
	if until != "" {
		q += ` AND anam_time <= ?`
		args = append(args, until)
	}
	q += ` ORDER BY anam_time, seq`

	rows, err := d.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("ops: changes_since: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []EventRow
	for rows.Next() {
		var ev EventRow
		var kind string
		if err := rows.Scan(&ev.Seq, &ev.AnamTime, &kind, &ev.FacetID, &ev.ArtifactVersion); err != nil {
			return nil, fmt.Errorf("ops: scan change: %w", err)
		}
		ev.Kind = facet.AnamEventKind(kind)
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ops: iterate changes: %w", err)
	}
	return out, nil
}
