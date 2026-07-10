// Package ledger is the DynamoDB half of Anam's store: the append-only record of
// what Anam knew (ARCHITECTURE.md §7, §8).
//
// This is the error kernel's substrate. Admit and Mark write an artifact and/or a
// record-event and the watermark bump in one TransactWriteItems, so a caller may
// crash at any moment and re-run to known-good: the transaction either happened or it
// did not, and re-running an attempt that happened reports ErrAlreadyWritten rather
// than writing twice.
//
// The ledger carries no secondary indexes. It serves exactly two access paths — the
// current version of one facet, and the record-events since a watermark — and both
// are strongly-consistent base-table range scans. Everything else a reader wants (the
// timeline, a type filter, changes_since) is an index, and indexes are derived, so
// they live in the operational store and rebuild by replay.
//
// Append-only is not enforced by DynamoDB — it cannot refuse an UpdateItem — so it is
// enforced here by construction (no method mutates or deletes) and guarded by a
// structural test that fails if this package ever names UpdateItem or DeleteItem.
package ledger

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	ddbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// Watermark is a position in the record-event stream. W is the seq of the most
// recent record-event; genesis is 0 and rec#0 never exists.
//
// It is a distinct type, not a bare int64, so that a watermark and a checkpoint
// version can never be swapped in a comparison without the compiler objecting — the
// same rung-3 move as facet.Canonical.
type Watermark int64

// The typed outcomes of a conflicting write. TransactionCanceledException reports its
// reasons positionally, and the three positions mean three different things; mapping
// them apart is what makes commit idempotent rather than merely retryable (§8).
var (
	// ErrAlreadyWritten: this exact artifact version already exists with the same
	// content hash. The previous attempt succeeded; treat as success.
	ErrAlreadyWritten = errors.New("ledger: artifact already written (idempotent no-op)")

	// ErrDivergentArtifact: this artifact version exists with a different content
	// hash. Someone wrote different bytes at the same identity — a lost update. Stop.
	ErrDivergentArtifact = errors.New("ledger: artifact version exists with different content")

	// ErrStaleWatermark: another writer advanced W after it was read. Re-read the
	// head, re-stamp record_time, and retry. The retry loop belongs to the caller —
	// the gate — never to this package.
	ErrStaleWatermark = errors.New("ledger: watermark advanced since read")

	// ErrCorrupt: the record-event's seq is taken while the watermark condition
	// passed — the counter is behind its own log. No write can be trusted; stop.
	ErrCorrupt = errors.New("ledger: corrupt — counter is behind its own record-events")

	// ErrSchemaMismatch: the table exists with a different key schema. A silently
	// divergent table is a corrupt kernel; refuse to touch it.
	ErrSchemaMismatch = errors.New("ledger: existing table has a different key schema")
)

// Ledger is a handle on one person's timeline in one table. Every method takes the
// caller's context and inherits its deadline: the store never sets its own, and an
// unreachable endpoint fails fast rather than hanging (§8).
type Ledger struct {
	client *dynamodb.Client
	table  string
	pk     string
}

// New connects to DynamoDB at endpoint. DynamoDB Local does not authenticate; the
// static credentials are literally "fake", and the region is nominal.
func New(ctx context.Context, endpoint, region, table, personID string) (*Ledger, error) {
	cfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider("fake", "fake", "")),
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: load aws config: %w", err)
	}
	client := dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String(endpoint)
	})
	return &Ledger{client: client, table: table, pk: personPK(personID)}, nil
}

// Init idempotently prepares the ledger: it creates the single table (pk hash, sk
// range, on-demand billing) and seeds the watermark counter at 0.
//
// The seed is not optional. The commit transaction advances the counter under the
// condition that it still holds the value read — and against a missing item that
// condition fails rather than creating it (verified against DynamoDB Local), so an
// unseeded table rejects every first commit forever.
//
// If the table already exists its key schema is compared, and a mismatch is
// ErrSchemaMismatch rather than a shrug: on a -sharedDb endpoint, stale tables from
// earlier schemas survive indefinitely.
func (l *Ledger) Init(ctx context.Context) error {
	_, err := l.client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:   aws.String(l.table),
		BillingMode: ddbtypes.BillingModePayPerRequest,
		AttributeDefinitions: []ddbtypes.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: ddbtypes.ScalarAttributeTypeS},
			{AttributeName: aws.String("sk"), AttributeType: ddbtypes.ScalarAttributeTypeS},
		},
		KeySchema: []ddbtypes.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: ddbtypes.KeyTypeHash},
			{AttributeName: aws.String("sk"), KeyType: ddbtypes.KeyTypeRange},
		},
	})
	var inUse *ddbtypes.ResourceInUseException
	switch {
	case err == nil:
	case errors.As(err, &inUse):
		if err := l.verifySchema(ctx); err != nil {
			return err
		}
	default:
		return fmt.Errorf("ledger: create table %q: %w", l.table, err)
	}

	return l.seedCounter(ctx)
}

// verifySchema compares an existing table's key schema against the one Init creates.
func (l *Ledger) verifySchema(ctx context.Context) error {
	out, err := l.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(l.table),
	})
	if err != nil {
		return fmt.Errorf("ledger: describe table %q: %w", l.table, err)
	}

	want := map[string]ddbtypes.KeyType{"pk": ddbtypes.KeyTypeHash, "sk": ddbtypes.KeyTypeRange}
	got := map[string]ddbtypes.KeyType{}
	for _, k := range out.Table.KeySchema {
		got[aws.ToString(k.AttributeName)] = k.KeyType
	}
	if len(got) != len(want) || got["pk"] != want["pk"] || got["sk"] != want["sk"] {
		return fmt.Errorf("%w: table %q has key schema %v", ErrSchemaMismatch, l.table, out.Table.KeySchema)
	}
	return nil
}

// seedCounter writes the watermark item at 0 unless it already exists.
func (l *Ledger) seedCounter(ctx context.Context) error {
	_, err := l.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(l.table),
		Item: map[string]ddbtypes.AttributeValue{
			"pk": &ddbtypes.AttributeValueMemberS{Value: l.pk},
			"sk": &ddbtypes.AttributeValueMemberS{Value: seqSK},
			"n":  &ddbtypes.AttributeValueMemberN{Value: "0"},
		},
		ConditionExpression: aws.String("attribute_not_exists(sk)"),
	})
	var ccf *ddbtypes.ConditionalCheckFailedException
	if errors.As(err, &ccf) {
		return nil // already seeded — idempotent
	}
	if err != nil {
		return fmt.Errorf("ledger: seed watermark counter: %w", err)
	}
	return nil
}

// Drop deletes the table. It exists for the rebuild test — drop, init, re-ingest,
// compare the fold (MVP.md §7) — and for test teardown. It deletes a table, never an
// item: append-only governs the items, and no method here mutates or removes one.
func (l *Ledger) Drop(ctx context.Context) error {
	_, err := l.client.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(l.table)})
	if err != nil {
		return fmt.Errorf("ledger: drop table %q: %w", l.table, err)
	}
	return nil
}

// Head reads the current watermark, strongly consistent.
func (l *Ledger) Head(ctx context.Context) (Watermark, error) {
	out, err := l.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: aws.String(l.table),
		Key: map[string]ddbtypes.AttributeValue{
			"pk": &ddbtypes.AttributeValueMemberS{Value: l.pk},
			"sk": &ddbtypes.AttributeValueMemberS{Value: seqSK},
		},
		ConsistentRead: aws.Bool(true),
	})
	if err != nil {
		return 0, fmt.Errorf("ledger: read watermark: %w", err)
	}
	if out.Item == nil {
		return 0, fmt.Errorf("ledger: watermark counter missing — table not initialised (run init)")
	}
	n, ok := out.Item["n"].(*ddbtypes.AttributeValueMemberN)
	if !ok {
		return 0, fmt.Errorf("ledger: watermark counter holds no number")
	}
	var w Watermark
	if _, err := fmt.Sscan(n.Value, &w); err != nil {
		return 0, fmt.Errorf("ledger: watermark %q is not an integer: %w", n.Value, err)
	}
	return w, nil
}
