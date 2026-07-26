package ledger

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/mattjoyce/anam/internal/facet"
)

// Admit appends a facet artifact together with its anam-event, and Mark appends a
// anam-event alone (a tombstone, an endorsement — status transitions that carry no
// new artifact). They are two calls rather than one with an optional artifact so that
// {no artifact, admitted} is unrepresentable (MVP.md §2).
//
// Both take the watermark the caller read. The event's Seq must be readW+1: the
// caller owns the seq because the caller stamps AnamTime, and the two must be
// assigned together — inside the caller's retry — so that seq order and anam-time
// order agree by construction (§8).
//
// Neither retries. On ErrStaleWatermark the caller re-reads the head, re-stamps, and
// tries again; the loop, its bound and its give-up belong to the gate, outside the
// kernel.

// Admit writes f, its anam-event, and the watermark bump in one transaction.
func (l *Ledger) Admit(ctx context.Context, readW Watermark, f facet.Facet, ev facet.AnamEvent) error {
	if err := checkSeq(readW, ev); err != nil {
		return err
	}
	fi, err := marshalItem(toFacetItem(l.pk, f))
	if err != nil {
		return err
	}
	ri, err := marshalItem(toAnamEventItem(l.pk, ev))
	if err != nil {
		return err
	}

	items := []ddbtypes.TransactWriteItem{
		{Put: &ddbtypes.Put{
			TableName:           aws.String(l.table),
			Item:                fi,
			ConditionExpression: aws.String("attribute_not_exists(sk)"),
			// The mechanism, not an optimisation: the cancellation carries the existing
			// artifact back, so the divergence check below needs no second read.
			ReturnValuesOnConditionCheckFailure: ddbtypes.ReturnValuesOnConditionCheckFailureAllOld,
		}},
		{Put: &ddbtypes.Put{
			TableName:           aws.String(l.table),
			Item:                ri,
			ConditionExpression: aws.String("attribute_not_exists(sk)"),
		}},
		l.bumpCounter(readW),
	}

	_, err = l.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err != nil {
		return l.mapCancellation(err, &f, admitArtifactPos)
	}
	return nil
}

// Mark writes an anam-event and the watermark bump in one transaction.
func (l *Ledger) Mark(ctx context.Context, readW Watermark, ev facet.AnamEvent) error {
	if err := checkSeq(readW, ev); err != nil {
		return err
	}
	ri, err := marshalItem(toAnamEventItem(l.pk, ev))
	if err != nil {
		return err
	}

	items := []ddbtypes.TransactWriteItem{
		{Put: &ddbtypes.Put{
			TableName:           aws.String(l.table),
			Item:                ri,
			ConditionExpression: aws.String("attribute_not_exists(sk)"),
		}},
		l.bumpCounter(readW),
	}

	_, err = l.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err != nil {
		return l.mapCancellation(err, nil, markArtifactPos)
	}
	return nil
}

// Commit pairs one artifact with the anam-event that admits it. It exists so that
// AdmitBatch cannot be handed an artifact without its event, or the reverse — the
// same reason Admit and Mark are two calls rather than one (MVP.md §2).
type Commit struct {
	Facet facet.Facet
	Event facet.AnamEvent
}

// MaxBatch is the largest number of facets one AdmitBatch may carry.
//
// TransactWriteItems accepts 100 items. Each facet costs two — its artifact and its
// anam-event — and the watermark bump costs one, so 49 facets is 99 items and 50
// would be 101. The cap is DynamoDB's, not a tuning choice.
const MaxBatch = 49

// AdmitBatch appends many artifacts, their anam-events, and one watermark bump in a
// single transaction, allocating a contiguous block of seqs.
//
// This does not weaken `W ≡ seq` (§8). Every anam-event still receives its own
// unique, monotonic, gapless seq — readW+1 through readW+len(cs) — and the counter
// lands on the last of them. What changes is only how many events one counter
// condition covers: the invariant is "every event has its own seq", never "one
// event per bump".
//
// What it does trade is contention tolerance for round trips. A stale watermark
// costs the whole batch rather than one facet, which is the right trade for a
// backfill (one writer, no contention) and the wrong one under many writers. The
// caller chooses the size; a batch of one is exactly Admit.
func (l *Ledger) AdmitBatch(ctx context.Context, readW Watermark, cs []Commit) error {
	if err := checkBatch(readW, cs); err != nil {
		return err
	}

	items := make([]ddbtypes.TransactWriteItem, 0, 2*len(cs)+1)
	for _, c := range cs {
		fi, err := marshalItem(toFacetItem(l.pk, c.Facet))
		if err != nil {
			return err
		}
		ri, err := marshalItem(toAnamEventItem(l.pk, c.Event))
		if err != nil {
			return err
		}
		items = append(items,
			ddbtypes.TransactWriteItem{Put: &ddbtypes.Put{
				TableName:                           aws.String(l.table),
				Item:                                fi,
				ConditionExpression:                 aws.String("attribute_not_exists(sk)"),
				ReturnValuesOnConditionCheckFailure: ddbtypes.ReturnValuesOnConditionCheckFailureAllOld,
			}},
			ddbtypes.TransactWriteItem{Put: &ddbtypes.Put{
				TableName:           aws.String(l.table),
				Item:                ri,
				ConditionExpression: aws.String("attribute_not_exists(sk)"),
			}},
		)
	}
	items = append(items, l.bumpCounterTo(readW, readW+Watermark(len(cs))))

	_, err := l.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err != nil {
		return l.mapBatchCancellation(err, cs)
	}
	return nil
}

// checkBatch rejects a batch the ledger cannot write soundly: the wrong size, seqs
// that do not form the contiguous block following readW, or two commits for the same
// artifact. The last one matters because DynamoDB rejects a transaction touching one
// key twice, and the error it returns says nothing about which facet.
func checkBatch(readW Watermark, cs []Commit) error {
	switch {
	case len(cs) == 0:
		return fmt.Errorf("ledger: empty batch")
	case len(cs) > MaxBatch:
		return fmt.Errorf("ledger: batch of %d exceeds MaxBatch %d", len(cs), MaxBatch)
	}
	seen := make(map[string]int, len(cs))
	for i, c := range cs {
		if want := int64(readW) + int64(i) + 1; c.Event.Seq != want {
			return fmt.Errorf("ledger: batch item %d has seq %d, want %d (block follows watermark %d)",
				i, c.Event.Seq, want, readW)
		}
		sk := facetSK(c.Facet.ID, c.Facet.ArtifactVersion)
		if first, dup := seen[sk]; dup {
			return fmt.Errorf("ledger: batch items %d and %d are both %s — one transaction cannot write a key twice",
				first, i, sk)
		}
		seen[sk] = i
	}
	return nil
}

func checkSeq(readW Watermark, ev facet.AnamEvent) error {
	if ev.Seq != int64(readW)+1 {
		return fmt.Errorf("ledger: event seq %d does not follow watermark %d", ev.Seq, readW)
	}
	return nil
}

// bumpCounter advances the watermark by one iff it still holds the value the caller
// read.
func (l *Ledger) bumpCounter(readW Watermark) ddbtypes.TransactWriteItem {
	return l.bumpCounterTo(readW, readW+1)
}

// bumpCounterTo advances the watermark to next iff it still holds readW.
func (l *Ledger) bumpCounterTo(readW, next Watermark) ddbtypes.TransactWriteItem {
	return ddbtypes.TransactWriteItem{Update: &ddbtypes.Update{
		TableName: aws.String(l.table),
		Key: map[string]ddbtypes.AttributeValue{
			"pk": &ddbtypes.AttributeValueMemberS{Value: l.pk},
			"sk": &ddbtypes.AttributeValueMemberS{Value: seqSK},
		},
		UpdateExpression:         aws.String("SET #n = :next"),
		ConditionExpression:      aws.String("#n = :read"),
		ExpressionAttributeNames: map[string]string{"#n": "n"},
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":next": &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", int64(next))},
			":read": &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", int64(readW))},
		},
	}}
}

// Transaction item positions. The cancellation reasons are positional, and the
// position is the meaning (§8).
const (
	admitArtifactPos = 0  // Admit: [artifact, anam-event, counter]
	markArtifactPos  = -1 // Mark: [anam-event, counter] — no artifact position
)

// mapCancellation turns a TransactWriteItems error into the typed outcome the caller
// acts on. The three arms are three different situations:
//
//   - the artifact's condition failed → this (facet_id, version) exists. Same
//     content hash: the previous attempt succeeded — ErrAlreadyWritten, treat as
//     success. Different hash: a lost update — ErrDivergentArtifact, stop.
//   - the counter's condition failed → another writer advanced W — ErrStaleWatermark,
//     re-read and retry (in the caller).
//   - the anam-event's condition failed while the counter's passed → the counter is
//     behind its own log — ErrCorrupt, stop. Nothing sound can be written.
//
// The arms are decided by precedence, not position order, because a stale writer
// fails two conditions at once: its anam-event's seq is taken AND the counter moved.
// That pair is the ordinary race, not corruption — ErrCorrupt is only the impossible
// remainder, a taken seq under a counter that had not moved.
//
// This mapping is the idempotency: collapse the arms into one "conflict" and the
// caller either retries a write that already succeeded or rejects a benign re-run.
func (l *Ledger) mapCancellation(err error, f *facet.Facet, artifactPos int) error {
	var tce *ddbtypes.TransactionCanceledException
	if !errors.As(err, &tce) {
		return fmt.Errorf("ledger: transact write: %w", err)
	}

	counterPos := len(tce.CancellationReasons) - 1
	failed := map[int]ddbtypes.CancellationReason{}
	for pos, r := range tce.CancellationReasons {
		if aws.ToString(r.Code) == "ConditionalCheckFailed" {
			failed[pos] = r
		}
	}

	if len(failed) == 0 {
		return fmt.Errorf("ledger: transaction cancelled without a failed condition: %w", err)
	}
	if r, ok := failed[artifactPos]; ok {
		return diverged(f, r)
	}
	if _, ok := failed[counterPos]; ok {
		return ErrStaleWatermark
	}
	return fmt.Errorf("%w: an anam-event seq is taken while the watermark condition passed", ErrCorrupt)
}

// BatchConflict reports which commits in a batch failed their artifact condition.
// Nothing was written — a cancelled transaction writes nothing — so the caller may
// drop the already-written entries and retry the remainder.
//
// The two lists are the same distinction diverged() draws for a single Admit, kept
// apart because a batch can hit both at once: AlreadyWritten is a benign re-run of a
// write that succeeded, Divergent is a lost update and must stop.
type BatchConflict struct {
	AlreadyWritten []int
	Divergent      []int
	Reasons        map[int]error
}

func (e *BatchConflict) Error() string {
	return fmt.Sprintf("ledger: batch cancelled — %d already written, %d divergent",
		len(e.AlreadyWritten), len(e.Divergent))
}

// mapBatchCancellation is mapCancellation's arms over a block of commits, decided by
// the same precedence: the counter first, because a stale writer fails its counter
// condition and some of its artifact conditions at once, and the counter is the arm
// that says "re-read and try again".
func (l *Ledger) mapBatchCancellation(err error, cs []Commit) error {
	var tce *ddbtypes.TransactionCanceledException
	if !errors.As(err, &tce) {
		return fmt.Errorf("ledger: transact write batch: %w", err)
	}

	failed := map[int]ddbtypes.CancellationReason{}
	for pos, r := range tce.CancellationReasons {
		if aws.ToString(r.Code) == "ConditionalCheckFailed" {
			failed[pos] = r
		}
	}
	if len(failed) == 0 {
		return fmt.Errorf("ledger: batch cancelled without a failed condition: %w", err)
	}
	if _, stale := failed[2*len(cs)]; stale {
		return ErrStaleWatermark
	}

	conflict := &BatchConflict{Reasons: map[int]error{}}
	for pos, r := range failed {
		if pos%2 != 0 { // an anam-event's seq is taken while the counter's condition held
			return fmt.Errorf("%w: an anam-event seq is taken while the watermark condition passed", ErrCorrupt)
		}
		i := pos / 2
		switch e := diverged(&cs[i].Facet, r); {
		case errors.Is(e, ErrAlreadyWritten):
			conflict.AlreadyWritten = append(conflict.AlreadyWritten, i)
		default:
			conflict.Divergent = append(conflict.Divergent, i)
			conflict.Reasons[i] = e
		}
	}
	sort.Ints(conflict.AlreadyWritten)
	sort.Ints(conflict.Divergent)
	return conflict
}

// diverged decides ErrAlreadyWritten vs ErrDivergentArtifact by comparing the
// existing item the cancellation carried back against the attempted write.
func diverged(f *facet.Facet, r ddbtypes.CancellationReason) error {
	if f == nil {
		return fmt.Errorf("%w: no artifact in this transaction yet its position failed", ErrCorrupt)
	}
	existing, ok := r.Item["content_hash"].(*ddbtypes.AttributeValueMemberS)
	if !ok {
		return fmt.Errorf("%w: existing item returned no content_hash to compare", ErrDivergentArtifact)
	}
	if existing.Value != f.Descriptor.ContentHash {
		return fmt.Errorf("%w: facet %s v%d holds %s, attempted %s",
			ErrDivergentArtifact, f.ID, f.ArtifactVersion, existing.Value, f.Descriptor.ContentHash)
	}
	// Same identity, same bytes: we are looking at our own successful earlier write.
	// Belt and braces: the payloads must agree too, or the hash lied.
	if p, ok := r.Item["payload"].(*ddbtypes.AttributeValueMemberB); ok {
		if !bytes.Equal(p.Value, f.Payload.Bytes()) {
			return fmt.Errorf("%w: content hash matches but payload bytes differ", ErrDivergentArtifact)
		}
	}
	return ErrAlreadyWritten
}
