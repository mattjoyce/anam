package ops_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 200: a migration reads the registered rows of one store in a transaction and
// writes them into another, moment of registration included, all or none.
func TestRegistered_ReadFromOneStoreIsWrittenToAnother(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	from, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = from.Close() }()
	for v := 1; v <= 2; v++ {
		if err := from.RegisterDefinition(ctx, ops.DefinitionRow{Type: "test.reading", Version: v, Body: `{"v":` + string(rune('0'+v)) + `}`}, "2026-10-2"+string(rune('0'+v))+"T00:00:00.000000000Z"); err != nil {
			t.Fatal(err)
		}
	}
	if err := from.RegisterProjection(ctx, ops.ProjectionRow{Name: "heart", Version: 1, Body: `{"name":"heart"}`}, "2026-10-23T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	// An inline projection, filed the way a query's receipt files it: addressed by the
	// hash of its own bytes.
	inlineBody := []byte(`{"max_classification":"Normal","statuses":["admitted"],"types":["test.reading"]}`)
	inlineHash, err := from.Put(ctx, ops.KindTemplateBody, inlineBody)
	if err != nil {
		t.Fatal(err)
	}
	if err := from.PutDefinitions(ctx, []ops.DefinitionRow{{Type: "bundled.type", Version: 1, Body: `{}`}}); err != nil {
		t.Fatal(err)
	}

	got, err := from.ReadRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Definitions) != 2 || len(got.Projections) != 1 || len(got.Provisioned) != 1 ||
		len(got.TemplateBodies) != 1 || got.TemplateBodies[0].Hash != inlineHash || !bytes.Equal(got.TemplateBodies[0].Body, inlineBody) || got.Definitions[1].RegisteredAt != "2026-10-22T00:00:00.000000000Z" {
		t.Fatalf("ReadRegistered = %+v", got)
	}

	to, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = to.Close() }()
	if err := to.CarryRegistered(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, err := to.ReadRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Definitions) != 2 || again.Definitions[0] != got.Definitions[0] || again.Projections[0] != got.Projections[0] ||
		len(again.TemplateBodies) != 1 || again.TemplateBodies[0].Hash != inlineHash {
		t.Errorf("the new store holds %+v, want the old store's rows", again)
	}
	if len(again.Provisioned) != 0 {
		t.Errorf("the provisioned Definitions were written: %+v; the daemon provisions its own", again.Provisioned)
	}

	// Writing the rows again fails whole: the tables are immutable and the call is
	// one transaction, so a failed carry leaves nothing half-written.
	extra := got
	extra.Definitions = append([]ops.RegisteredDefinition{{DefinitionRow: ops.DefinitionRow{Type: "test.other", Version: 1, Body: `{}`}, RegisteredAt: "x"}}, got.Definitions...)
	if err := to.CarryRegistered(ctx, extra); err == nil {
		t.Fatal("a carry over rows already there succeeded")
	}
	after, err := to.ReadRegistered(ctx)
	if err != nil || len(after.Definitions) != 2 {
		t.Errorf("after the failed carry the store holds %d Definitions (%v), want the 2 it had", len(after.Definitions), err)
	}
}

// Card 211: an inline projection body is carried under the hash that addresses it. A
// body that does not hash to its key is refused whole, the same way a Get would
// refuse it, so the new store never holds a row its receipts cannot trust.
func TestCarryRegistered_RefusesAnInlineBodyThatDoesNotHashToItsKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	to, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = to.Close() }()
	body := []byte(`{"types":[]}`)
	bad := ops.Registered{TemplateBodies: []ops.TemplateBody{{Hash: ops.Digest([]byte("other")), Body: body}}}
	if err := to.CarryRegistered(ctx, bad); !errors.Is(err, ops.ErrCorrupt) {
		t.Fatalf("CarryRegistered of a body under another hash = %v, want ErrCorrupt", err)
	}
	if got, _ := to.List(ctx, ops.KindTemplateBody); len(got) != 0 {
		t.Errorf("a refused carry left %d inline rows", len(got))
	}
}

// Card 224: a schema bump carries the whole authored tier. A store with rows of every
// authored kind is read, a fresh store is initialised, the rows are carried into it,
// and every row comes back byte-identical: the registered Definitions and projections,
// the inline projection bodies, the admission holds, and the receipts, served values,
// saved selections and selection codes.
func TestCarryRegistered_EveryAuthoredRowComesBackByteIdentical(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	from, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = from.Close() }()
	if err := from.RegisterDefinition(ctx, ops.DefinitionRow{Type: "test.reading", Version: 1, Body: `{"v":1}`}, "2026-10-22T00:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if err := from.RegisterProjection(ctx, ops.ProjectionRow{Name: "heart", Version: 1, Body: `{"name":"heart"}`}, "2026-10-22T01:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	if err := from.SetHold(ctx, "scale", "2026-10-22T02:00:00.000000000Z"); err != nil {
		t.Fatal(err)
	}
	bodies := map[string][][]byte{
		ops.KindTemplateBody:  {[]byte(`{"types":["test.reading"]}`)},
		ops.KindReceipt:       {[]byte(`{"served_hash":"a"}`), []byte(`{"served_hash":"b","principal":"assistant"}`)},
		ops.KindServed:        {[]byte("served value one\x00\xff"), []byte(`[{"facet_id":"f"}]`)},
		ops.KindSelection:     {[]byte("pick(E) :- entry(E, _, _, _, _, _, _).")},
		ops.KindSelectionCode: {[]byte(`{"code":"0123abcd9","hash":"sha256:x"}`)},
	}
	for kind, list := range bodies {
		for _, b := range list {
			if _, err := from.Put(ctx, kind, b); err != nil {
				t.Fatalf("put %s: %v", kind, err)
			}
		}
	}
	// A checkpoint value is derived and is not carried: a mint makes it again.
	if _, err := from.Put(ctx, ops.KindCheckpoint, []byte("a checkpoint value")); err != nil {
		t.Fatal(err)
	}

	got, err := from.ReadRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	to, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = to.Close() }()
	if err := to.CarryRegistered(ctx, got); err != nil {
		t.Fatal(err)
	}

	again, err := to.ReadRegistered(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got.Provisioned, again.Provisioned = nil, nil // the daemon provisions its own
	if !reflect.DeepEqual(again, got) {
		t.Errorf("the carried store reads back\n%+v\nwant\n%+v", again, got)
	}
	for kind, list := range bodies {
		for _, b := range list {
			back, err := to.Get(ctx, kind, ops.Digest(b))
			if err != nil || !bytes.Equal(back, b) {
				t.Errorf("%s %q came back as %q (%v)", kind, b, back, err)
			}
		}
	}
	if got, _ := to.List(ctx, ops.KindCheckpoint); len(got) != 0 {
		t.Errorf("the carry wrote %d checkpoint values; they are derived", len(got))
	}
}

// A content row whose bytes do not hash to its key is refused whole, as an inline body
// is: the new store never holds a receipt or a served value a read would refuse.
func TestCarryRegistered_RefusesContentThatDoesNotHashToItsKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	to, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = to.Close() }()
	good := []byte(`{"served_hash":"a"}`)
	bad := ops.Registered{Content: []ops.Content{
		{Kind: ops.KindReceipt, Hash: ops.Digest(good), Body: good},
		{Kind: ops.KindServed, Hash: ops.Digest([]byte("other")), Body: []byte("served")},
	}}
	if err := to.CarryRegistered(ctx, bad); !errors.Is(err, ops.ErrCorrupt) {
		t.Fatalf("CarryRegistered of a served value under another hash = %v, want ErrCorrupt", err)
	}
	if got, _ := to.List(ctx, ops.KindReceipt); len(got) != 0 {
		t.Errorf("a refused carry left %d receipts", len(got))
	}
}
