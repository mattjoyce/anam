package ledger

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/mattjoyce/anam/internal/facet"
)

// Admit appends a facet artifact together with its record-event, and Mark appends a
// record-event alone (a tombstone, an endorsement — status transitions that carry no
// new artifact). They are two calls rather than one with an optional artifact so that
// {no artifact, admitted} is unrepresentable (MVP.md §2).
//
// Both take the watermark the caller read. The event's Seq must be readW+1: the
// caller owns the seq because the caller stamps RecordTime, and the two must be
// assigned together — inside the caller's retry — so that seq order and record-time
// order agree by construction (§8).
//
// Neither retries. On ErrStaleWatermark the caller re-reads the head, re-stamps, and
// tries again; the loop, its bound and its give-up belong to the gate, outside the
// kernel.

// Admit writes f, its record-event, and the watermark bump in one transaction.
func (l *Ledger) Admit(ctx context.Context, readW Watermark, f facet.Facet, ev facet.RecordEvent) error {
	if err := checkSeq(readW, ev); err != nil {
		return err
	}
	fi, err := marshalItem(toFacetItem(l.pk, f))
	if err != nil {
		return err
	}
	ri, err := marshalItem(toRecordItem(l.pk, ev))
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

// Mark writes a record-event and the watermark bump in one transaction.
func (l *Ledger) Mark(ctx context.Context, readW Watermark, ev facet.RecordEvent) error {
	if err := checkSeq(readW, ev); err != nil {
		return err
	}
	ri, err := marshalItem(toRecordItem(l.pk, ev))
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

func checkSeq(readW Watermark, ev facet.RecordEvent) error {
	if ev.Seq != int64(readW)+1 {
		return fmt.Errorf("ledger: event seq %d does not follow watermark %d", ev.Seq, readW)
	}
	return nil
}

// bumpCounter advances the watermark iff it still holds the value the caller read.
func (l *Ledger) bumpCounter(readW Watermark) ddbtypes.TransactWriteItem {
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
			":next": &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", int64(readW)+1)},
			":read": &ddbtypes.AttributeValueMemberN{Value: fmt.Sprintf("%d", int64(readW))},
		},
	}}
}

// Transaction item positions. The cancellation reasons are positional, and the
// position is the meaning (§8).
const (
	admitArtifactPos = 0  // Admit: [artifact, record-event, counter]
	markArtifactPos  = -1 // Mark: [record-event, counter] — no artifact position
)

// mapCancellation turns a TransactWriteItems error into the typed outcome the caller
// acts on. The three arms are three different situations:
//
//   - the artifact's condition failed → this (facet_id, version) exists. Same
//     content hash: the previous attempt succeeded — ErrAlreadyWritten, treat as
//     success. Different hash: a lost update — ErrDivergentArtifact, stop.
//   - the counter's condition failed → another writer advanced W — ErrStaleWatermark,
//     re-read and retry (in the caller).
//   - the record-event's condition failed while the counter's passed → the counter is
//     behind its own log — ErrCorrupt, stop. Nothing sound can be written.
//
// The arms are decided by precedence, not position order, because a stale writer
// fails two conditions at once: its record-event's seq is taken AND the counter moved.
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
	return fmt.Errorf("%w: a record-event seq is taken while the watermark condition passed", ErrCorrupt)
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
