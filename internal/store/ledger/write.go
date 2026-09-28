package ledger

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"

	sqlite3 "modernc.org/sqlite/lib"

	"github.com/mattjoyce/anam/internal/facet"
)

// Admit appends a facet artifact together with its anam-event, and Mark appends an
// anam-event alone (a tombstone, an endorsement: status transitions that carry no
// new artifact). They are two calls, not one with an optional artifact, so that
// {no artifact, admitted} is unrepresentable (MVP.md §2).
//
// Both take the watermark the caller read. The event's Seq must be readW+1: the
// caller owns the seq because the caller stamps AnamTime, and the two must be
// assigned together, inside the caller's retry, so that seq order and anam-time
// order agree by construction (§8).
//
// Neither retries. On ErrStaleWatermark the caller re-reads the head, re-stamps, and
// tries again; the loop, its bound and its give-up belong to the gate, outside the
// kernel.

// Admit writes f and its anam-event in one transaction.
//
// It is AdmitBatch of one, not a second implementation of it. The one thing it does on
// its own is unwrap: a batch reports conflicts by position, through *BatchConflict,
// and with a single commit there is exactly one position, so callers keep the sentinel
// they can test against.
func (l *Ledger) Admit(ctx context.Context, readW Watermark, f facet.Facet, ev facet.AnamEvent) error {
	err := l.AdmitBatch(ctx, readW, []Commit{{Facet: f, Event: ev}})

	var conflict *BatchConflict
	if !errors.As(err, &conflict) {
		return err
	}
	if len(conflict.Divergent) == 1 {
		return conflict.Reasons[conflict.Divergent[0]]
	}
	return ErrAlreadyWritten
}

// Mark writes an anam-event in one transaction.
func (l *Ledger) Mark(ctx context.Context, readW Watermark, ev facet.AnamEvent) error {
	if err := checkSeq(readW, ev); err != nil {
		return err
	}
	return l.write(ctx, readW, func(tx *sql.Tx) error {
		return refused("anam-event", insertEvent(ctx, tx, ev))
	})
}

// Commit pairs one artifact with the anam-event that admits it. It exists so that
// AdmitBatch cannot be handed an artifact without its event, or the reverse.
type Commit struct {
	Facet facet.Facet
	Event facet.AnamEvent
}

// AdmitBatch appends many artifacts and their anam-events in a single transaction,
// over a contiguous block of seqs.
//
// This does not weaken `W ≡ seq` (§8). Every anam-event still receives its own
// unique, monotonic, gapless seq, readW+1 through readW+len(cs), and the head lands
// on the last of them.
//
// The ledger sets no limit on the size of a batch. How many events may share one
// transaction, and so one anam-time, is the gate's policy (gate.MaxBatch).
func (l *Ledger) AdmitBatch(ctx context.Context, readW Watermark, cs []Commit) error {
	if err := checkBatch(readW, cs); err != nil {
		return err
	}
	return l.write(ctx, readW, func(tx *sql.Tx) error {
		conflict, err := conflicts(ctx, tx, cs)
		if err != nil {
			return err
		}
		if conflict != nil {
			return conflict
		}
		for _, c := range cs {
			if err := refused("artifact", insertFacet(ctx, tx, c.Facet)); err != nil {
				return err
			}
			if err := refused("anam-event", insertEvent(ctx, tx, c.Event)); err != nil {
				return err
			}
		}
		return nil
	})
}

// write runs fn inside one transaction that holds the file's write lock from its
// first statement, after checking that the head is still the watermark the caller
// read. Anything but a nil from fn rolls the whole transaction back, so a refused
// batch writes nothing.
//
// The stale check comes first on purpose. A writer that read an old head may also be
// re-presenting artifacts that landed since, and "re-read and try again" is the answer
// that lets the caller sort those out against the head as it now is.
func (l *Ledger) write(ctx context.Context, readW Watermark, fn func(*sql.Tx) error) error {
	tx, err := l.db.BeginTx(ctx, nil)
	if err != nil {
		return classify("begin write", err)
	}
	defer func() { _ = tx.Rollback() }() // no-op after Commit

	h, err := head(ctx, tx)
	if err != nil {
		return err
	}
	if h != readW {
		return fmt.Errorf("%w: caller read %d, head is %d", ErrStaleWatermark, readW, h)
	}
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return classify("commit write", err)
	}
	return nil
}

// refused maps an insert's error. The checks that ran before the insert (the head, the
// seq block, the existing artifacts) cover everything the schema's constraints
// enforce, and they ran under the same write lock. A constraint failure here therefore
// means the checks and the database disagree, which is ErrCorrupt and not a conflict
// the caller can retry.
func refused(what string, err error) error {
	if err == nil {
		return nil
	}
	if code, ok := primaryCode(err); ok && code == sqlite3.SQLITE_CONSTRAINT {
		return fmt.Errorf("%w: %s: %v", ErrCorrupt, what, err)
	}
	return classify("insert "+what, err)
}

// checkBatch rejects a batch the ledger cannot write soundly: an empty one, seqs that
// do not form the contiguous block following readW, or two commits for the same
// artifact.
func checkBatch(readW Watermark, cs []Commit) error {
	if len(cs) == 0 {
		return fmt.Errorf("ledger: empty batch")
	}
	type key struct {
		id      string
		version int
	}
	seen := make(map[key]int, len(cs))
	for i, c := range cs {
		if want := int64(readW) + int64(i) + 1; c.Event.Seq != want {
			return fmt.Errorf("ledger: batch item %d has seq %d, want %d (block follows watermark %d)",
				i, c.Event.Seq, want, readW)
		}
		k := key{c.Facet.ID, c.Facet.ArtifactVersion}
		if first, dup := seen[k]; dup {
			return fmt.Errorf("ledger: batch items %d and %d are both %s v%d; one transaction cannot write an artifact twice",
				first, i, k.id, k.version)
		}
		seen[k] = i
	}
	return nil
}

func checkSeq(readW Watermark, ev facet.AnamEvent) error {
	if ev.Seq != int64(readW)+1 {
		return fmt.Errorf("ledger: event seq %d does not follow watermark %d", ev.Seq, readW)
	}
	return nil
}

// BatchConflict reports which commits in a batch name an artifact that already
// exists. Nothing was written, so the caller may drop the already-written entries and
// retry the remainder.
//
// The two lists are kept apart because a batch can hit both at once: AlreadyWritten is
// a benign re-run of a write that succeeded, Divergent is a lost update and must stop.
type BatchConflict struct {
	AlreadyWritten []int
	Divergent      []int
	Reasons        map[int]error
}

func (e *BatchConflict) Error() string {
	return fmt.Sprintf("ledger: batch refused, %d already written and %d divergent",
		len(e.AlreadyWritten), len(e.Divergent))
}

// conflicts looks up every artifact the batch is about to write and reports the ones
// that exist, in batch order. It returns nil when none do.
//
// This classifies; it does not enforce. The schema would refuse the overwrite anyway.
// What the schema cannot say is whether the existing row is our own earlier write or
// somebody else's different bytes, and that distinction is the idempotency: collapse
// the two and the caller either retries a write that already succeeded or accepts a
// lost update.
func conflicts(ctx context.Context, tx *sql.Tx, cs []Commit) (*BatchConflict, error) {
	stmt, err := tx.PrepareContext(ctx,
		`SELECT content_hash, payload FROM facet WHERE facet_id = ? AND artifact_version = ?`)
	if err != nil {
		return nil, classify("prepare conflict lookup", err)
	}
	defer func() { _ = stmt.Close() }()

	var conflict *BatchConflict
	for i, c := range cs {
		var (
			hash    string
			payload []byte
		)
		err := stmt.QueryRowContext(ctx, c.Facet.ID, c.Facet.ArtifactVersion).Scan(&hash, &payload)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, classify("look up "+c.Facet.ID, err)
		}

		if conflict == nil {
			conflict = &BatchConflict{Reasons: map[int]error{}}
		}
		if reason := diverged(c.Facet, hash, payload); reason != nil {
			conflict.Divergent = append(conflict.Divergent, i)
			conflict.Reasons[i] = reason
			continue
		}
		conflict.AlreadyWritten = append(conflict.AlreadyWritten, i)
	}
	return conflict, nil
}

// diverged compares an existing artifact against the attempted write. It returns nil
// when they are the same bytes, and ErrDivergentArtifact when they are not.
func diverged(f facet.Facet, existingHash string, existingPayload []byte) error {
	if existingHash != f.Descriptor.ContentHash {
		return fmt.Errorf("%w: facet %s v%d holds %s, attempted %s",
			ErrDivergentArtifact, f.ID, f.ArtifactVersion, existingHash, f.Descriptor.ContentHash)
	}
	// Same identity, same hash: we are looking at our own successful earlier write.
	// The payloads must agree too, or the hash lied.
	if !bytes.Equal(existingPayload, f.Payload.Bytes()) {
		return fmt.Errorf("%w: facet %s v%d: content hash matches but payload bytes differ",
			ErrDivergentArtifact, f.ID, f.ArtifactVersion)
	}
	return nil
}
