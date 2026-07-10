package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/mattjoyce/anam/internal/facet"
)

// ErrNotFound is returned by CurrentVersion for a facet with no artifact.
var ErrNotFound = errors.New("ledger: facet not found")

// The ledger's two read paths. Both are strongly consistent, because both feed the
// error kernel: CurrentVersion is what commit compares content hashes against, and
// Replay is what rebuilds every derived index. An eventually-consistent answer to
// either is a wrong commit or a wrong index, silently.

// CurrentVersion returns the highest artifact version of one logical facet.
//
// This is the lookup the identity-first key exists for: begins_with on the facet's
// own prefix, descending, limit 1. It needs no index and cannot be misled by a moved
// event_time — identity is the key, and values stay out of it (§8).
func (l *Ledger) CurrentVersion(ctx context.Context, facetID string) (facet.Facet, error) {
	out, err := l.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              aws.String(l.table),
		KeyConditionExpression: aws.String("pk = :pk AND begins_with(sk, :pfx)"),
		ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
			":pk":  &ddbtypes.AttributeValueMemberS{Value: l.pk},
			":pfx": &ddbtypes.AttributeValueMemberS{Value: facetPrefix(facetID)},
		},
		ScanIndexForward: aws.Bool(false),
		Limit:            aws.Int32(1),
		ConsistentRead:   aws.Bool(true),
	})
	if err != nil {
		return facet.Facet{}, fmt.Errorf("ledger: query current version of %s: %w", facetID, err)
	}
	if len(out.Items) == 0 {
		return facet.Facet{}, fmt.Errorf("%w: %s", ErrNotFound, facetID)
	}

	var it facetItem
	if err := attributevalue.UnmarshalMap(out.Items[0], &it); err != nil {
		return facet.Facet{}, fmt.Errorf("ledger: unmarshal facet item: %w", err)
	}
	return fromFacetItem(it)
}

// Replay returns the record-events in (sinceW, uptoW], in seq order — the range-scan
// that every derived state is rebuilt from (§7). Pass the head watermark as uptoW for
// "everything since"; pass a historical watermark for an as-of bound.
func (l *Ledger) Replay(ctx context.Context, sinceW, uptoW Watermark) ([]facet.RecordEvent, error) {
	if uptoW <= sinceW {
		return nil, nil
	}

	var events []facet.RecordEvent
	var startKey map[string]ddbtypes.AttributeValue
	for {
		out, err := l.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(l.table),
			KeyConditionExpression: aws.String("pk = :pk AND sk BETWEEN :lo AND :hi"),
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
				":pk": &ddbtypes.AttributeValueMemberS{Value: l.pk},
				":lo": &ddbtypes.AttributeValueMemberS{Value: recSK(int64(sinceW) + 1)},
				":hi": &ddbtypes.AttributeValueMemberS{Value: recSK(int64(uptoW))},
			},
			ConsistentRead:    aws.Bool(true),
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, fmt.Errorf("ledger: replay (%d, %d]: %w", sinceW, uptoW, err)
		}

		for _, raw := range out.Items {
			var it recordItem
			if err := attributevalue.UnmarshalMap(raw, &it); err != nil {
				return nil, fmt.Errorf("ledger: unmarshal record item: %w", err)
			}
			ev, err := fromRecordItem(it)
			if err != nil {
				return nil, err
			}
			events = append(events, ev)
		}

		if out.LastEvaluatedKey == nil {
			return events, nil
		}
		startKey = out.LastEvaluatedKey
	}
}
