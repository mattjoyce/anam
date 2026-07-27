package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

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

// MaxProbe is the largest number of facet ids one KnownFacets call may carry —
// BatchGetItem's own limit, not a tuning choice.
const MaxProbe = 100

// KnownFacets reports which of the given facet ids the ledger has ever admitted.
//
// It is a **fast negative**, and only that. Absent means the facet is unknown and
// its first artifact will be version 1 — certain, because versions start at 1 and an
// artifact is never deleted, so v1 exists for anything that has ever been admitted.
// Present means only that v1 exists; which version is *current* still needs
// CurrentVersion.
//
// That asymmetry is the whole point. A backfill is almost entirely first sightings,
// so one BatchGetItem answers the disposition of a whole batch and the per-facet
// query is never issued. A re-ingest is almost entirely present, falls back to the
// queries, and pays one wasted round trip for the correctness of the check.
//
// The reads are strongly consistent for the same reason CurrentVersion is: this
// feeds the commit decision, and an eventually-consistent "unknown" would admit a
// duplicate v1 over an existing facet.
func (l *Ledger) KnownFacets(ctx context.Context, facetIDs []string) (map[string]bool, error) {
	known := make(map[string]bool, len(facetIDs))
	if len(facetIDs) == 0 {
		return known, nil
	}
	if len(facetIDs) > MaxProbe {
		return nil, fmt.Errorf("ledger: probe of %d ids exceeds MaxProbe %d", len(facetIDs), MaxProbe)
	}

	// De-duplicate: BatchGetItem rejects a request naming one key twice.
	keys := make([]map[string]ddbtypes.AttributeValue, 0, len(facetIDs))
	seen := make(map[string]bool, len(facetIDs))
	for _, id := range facetIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		keys = append(keys, map[string]ddbtypes.AttributeValue{
			"pk": &ddbtypes.AttributeValueMemberS{Value: l.pk},
			"sk": &ddbtypes.AttributeValueMemberS{Value: facetSK(id, 1)},
		})
	}

	// UnprocessedKeys is DynamoDB throttling, not an error: the unread keys come
	// back to be asked for again. Treating them as absent would admit a duplicate.
	//
	// So the re-ask is required — but it is a retry, and a retry needs a bound and a
	// pause. Without them, sustained throttling is a hot loop against the thing already
	// asking to be left alone, and the only thing that ends it is the caller's deadline
	// firing somewhere else. Give up instead, saying so: a probe that cannot complete
	// must fail, because its "unknown" is what admits a first artifact.
	for attempt := 0; len(keys) > 0; attempt++ {
		if attempt == maxProbeAttempts {
			return nil, fmt.Errorf("%w: %d ids still unread after %d attempts",
				ErrThrottled, len(keys), maxProbeAttempts)
		}
		if attempt > 0 {
			if err := pause(ctx, probeBackoff(attempt)); err != nil {
				return nil, err
			}
		}

		out, err := l.client.BatchGetItem(ctx, &dynamodb.BatchGetItemInput{
			RequestItems: map[string]ddbtypes.KeysAndAttributes{
				l.table: {
					Keys:                 keys,
					ConsistentRead:       aws.Bool(true),
					ProjectionExpression: aws.String("id"),
				},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("ledger: probe known facets: %w", err)
		}
		for _, it := range out.Responses[l.table] {
			id, ok := it["id"].(*ddbtypes.AttributeValueMemberS)
			if !ok {
				return nil, fmt.Errorf("ledger: probed artifact carries no id")
			}
			known[id.Value] = true
		}
		keys = out.UnprocessedKeys[l.table].Keys
	}
	return known, nil
}

// ErrThrottled is returned when a probe still has unread keys after every attempt.
var ErrThrottled = errors.New("ledger: probe gave up under throttling")

// maxProbeAttempts bounds the re-ask. Six attempts span roughly 1.6s of backoff, which
// is longer than a healthy table ever needs and short enough to surface as a failure
// rather than as a call that never returns.
const maxProbeAttempts = 6

// probeBackoff is the pause before attempt n (n >= 1): 50ms doubling to 800ms. No
// jitter, because this is a single-writer backfill — there is no thundering herd to
// spread, only one caller being asked to slow down.
func probeBackoff(attempt int) time.Duration {
	d := 50 * time.Millisecond << (attempt - 1)
	if d > 800*time.Millisecond {
		d = 800 * time.Millisecond
	}
	return d
}

// pause waits d, or returns early if the caller's deadline fires first. A sleep that
// ignored the context would outlive the request it belongs to.
func pause(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("ledger: probe backoff: %w", ctx.Err())
	case <-t.C:
		return nil
	}
}

// CurrentVersion returns the highest artifact version of one logical facet.
//
// This is the lookup the identity-first key exists for: begins_with on the facet's
// own prefix, descending, limit 1. It needs no index and cannot be misled by a moved
// fact_time — identity is the key, and values stay out of it (§8).
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

// Version returns one exact artifact version of one logical facet.
//
// CurrentVersion answers "the latest", which is the right question for the gate — it
// compares content hashes against what is in force. It is the wrong question for a
// read at a historical watermark, where the fold has already derived *which* version
// was in force then, and the latest may be two supersessions further on.
//
// Because an artifact version is immutable once written, this lookup cannot be raced:
// the answer to "give me v2 of this facet" is the same whether ingest is idle or busy.
// That is what lets the as-of read run without a retry (see fold.ReadAt).
func (l *Ledger) Version(ctx context.Context, facetID string, version int) (facet.Facet, error) {
	if version < 1 {
		return facet.Facet{}, fmt.Errorf("ledger: artifact version %d is not a version", version)
	}
	out, err := l.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(l.table),
		Key: map[string]ddbtypes.AttributeValue{
			"pk": &ddbtypes.AttributeValueMemberS{Value: l.pk},
			"sk": &ddbtypes.AttributeValueMemberS{Value: facetSK(facetID, version)},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return facet.Facet{}, fmt.Errorf("ledger: get %s v%d: %w", facetID, version, err)
	}
	if out.Item == nil {
		return facet.Facet{}, fmt.Errorf("%w: %s v%d", ErrNotFound, facetID, version)
	}

	var it facetItem
	if err := attributevalue.UnmarshalMap(out.Item, &it); err != nil {
		return facet.Facet{}, fmt.Errorf("ledger: unmarshal facet item: %w", err)
	}
	return fromFacetItem(it)
}

// Replay returns the anam-events in (sinceW, uptoW], in seq order — the range-scan
// that every derived state is rebuilt from (§7). Pass the head watermark as uptoW for
// "everything since"; pass a historical watermark for an as-of bound.
func (l *Ledger) Replay(ctx context.Context, sinceW, uptoW Watermark) ([]facet.AnamEvent, error) {
	if uptoW <= sinceW {
		return nil, nil
	}

	var events []facet.AnamEvent
	var startKey map[string]ddbtypes.AttributeValue
	for {
		out, err := l.client.Query(ctx, &dynamodb.QueryInput{
			TableName:              aws.String(l.table),
			KeyConditionExpression: aws.String("pk = :pk AND sk BETWEEN :lo AND :hi"),
			ExpressionAttributeValues: map[string]ddbtypes.AttributeValue{
				":pk": &ddbtypes.AttributeValueMemberS{Value: l.pk},
				":lo": &ddbtypes.AttributeValueMemberS{Value: anamSK(int64(sinceW) + 1)},
				":hi": &ddbtypes.AttributeValueMemberS{Value: anamSK(int64(uptoW))},
			},
			ConsistentRead:    aws.Bool(true),
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, fmt.Errorf("ledger: replay (%d, %d]: %w", sinceW, uptoW, err)
		}

		for _, raw := range out.Items {
			var it anamEventItem
			if err := attributevalue.UnmarshalMap(raw, &it); err != nil {
				return nil, fmt.Errorf("ledger: unmarshal record item: %w", err)
			}
			ev, err := fromAnamEventItem(it)
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
