package ops_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/mattjoyce/anam/internal/receipt"
	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

// Content addressed rows (card 215): the receipts, served values, selections and
// checkpoint values that were files, over a real operational store file.

func TestPut_FilesTheBytesUnderTheirOwnDigest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	body := []byte(`[{"facet_id":"f1"}]`)
	for _, kind := range ops.Kinds() {
		got, err := d.Put(ctx, kind, body)
		if err != nil {
			t.Fatalf("Put(%s): %v", kind, err)
		}
		if got != ops.Digest(body) || !strings.HasPrefix(got, "sha256:") || len(got) != 71 {
			t.Errorf("Put(%s) = %q, want the sha256 of the bytes, %q", kind, got, ops.Digest(body))
		}
		back, err := d.Get(ctx, kind, got)
		if err != nil || !bytes.Equal(back, body) {
			t.Errorf("Get(%s) = %q, %v, want the bytes put", kind, back, err)
		}
	}
}

// Identical bytes have an identical key, so a re-put is a no-op rather than a second
// row. Serving the same projection twice must not litter the tier.
func TestPut_Idempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	body := []byte("the same bytes")
	first, err := d.Put(ctx, ops.KindReceipt, body)
	if err != nil {
		t.Fatal(err)
	}
	second, err := d.Put(ctx, ops.KindReceipt, body)
	if err != nil || second != first {
		t.Fatalf("second Put = %q, %v, want %q", second, err, first)
	}
	if got, err := d.List(ctx, ops.KindReceipt); err != nil || len(got) != 1 {
		t.Fatalf("List = %v, %v, want the one row", got, err)
	}
}

// A row that exists holding different bytes is a corruption of the table, not an
// update: content addressing means the key could only match if the bytes did.
func TestPut_RefusesDifferentBytesAtTheSameKey(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	body := []byte("original")
	digest, err := d.Put(ctx, ops.KindServed, body)
	if err != nil {
		t.Fatal(err)
	}
	if err := opstest.Corrupt(ctx, d, ops.KindServed, digest, []byte("altered")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Put(ctx, ops.KindServed, body); !errors.Is(err, ops.ErrCorrupt) {
		t.Fatalf("Put over altered bytes: %v, want ErrCorrupt", err)
	}
}

// Get verifies on the way out; Raw hands the bytes back so a verifier can say what
// they hash to.
func TestGet_DetectsTamperedBytes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	for _, kind := range ops.Kinds() {
		digest, err := d.Put(ctx, kind, []byte("as stored"))
		if err != nil {
			t.Fatal(err)
		}
		if err := opstest.Corrupt(ctx, d, kind, digest, []byte("as altered")); err != nil {
			t.Fatal(err)
		}
		if _, err := d.Get(ctx, kind, digest); !errors.Is(err, ops.ErrCorrupt) {
			t.Errorf("Get(%s) over altered bytes: %v, want ErrCorrupt", kind, err)
		}
		raw, err := d.Raw(ctx, kind, digest)
		if err != nil || string(raw) != "as altered" {
			t.Errorf("Raw(%s) = %q, %v, want the altered bytes unchecked", kind, raw, err)
		}
	}
}

func TestGetAndSize_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	absent := ops.Digest([]byte("never stored"))
	if _, err := d.Get(ctx, ops.KindReceipt, absent); !errors.Is(err, ops.ErrNotFound) {
		t.Errorf("Get(absent) = %v, want ErrNotFound", err)
	}
	if _, err := d.Size(ctx, ops.KindReceipt, absent); !errors.Is(err, ops.ErrNotFound) {
		t.Errorf("Size(absent) = %v, want ErrNotFound", err)
	}
	digest, err := d.Put(ctx, ops.KindReceipt, []byte("twelve bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if n, err := d.Size(ctx, ops.KindReceipt, digest); err != nil || n != 12 {
		t.Errorf("Size = %d, %v, want 12", n, err)
	}
	// A kind keeps its own rows: the same digest under another is not found.
	if _, err := d.Get(ctx, ops.KindServed, digest); !errors.Is(err, ops.ErrNotFound) {
		t.Errorf("Get under another kind = %v, want ErrNotFound", err)
	}
}

// Names reach the store from a command line and an HTTP path. One that is not a kind or
// a digest is refused before any query is made.
func TestRejectsMalformedNames(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	for _, tc := range []struct{ kind, digest string }{
		{"../escape", ops.Digest(nil)},
		{"Receipt", ops.Digest(nil)},
		{"", ops.Digest(nil)},
		{"receipt", "sha256:short"},
		{"receipt", "sha256:" + strings.Repeat("G", 64)},
		{"receipt", strings.Repeat("a", 64)},
		{"receipt", "x'; DROP TABLE receipt; --"},
	} {
		if _, err := d.Get(ctx, tc.kind, tc.digest); !errors.Is(err, ops.ErrInvalid) {
			t.Errorf("Get(%q, %q) = %v, want ErrInvalid", tc.kind, tc.digest, err)
		}
		if _, err := d.Size(ctx, tc.kind, tc.digest); !errors.Is(err, ops.ErrInvalid) {
			t.Errorf("Size(%q, %q) = %v, want ErrInvalid", tc.kind, tc.digest, err)
		}
	}
	if _, err := d.Put(ctx, "nonsense", []byte("x")); !errors.Is(err, ops.ErrInvalid) {
		t.Errorf("Put under an unknown kind = %v, want ErrInvalid", err)
	}
	if _, err := d.List(ctx, "nonsense"); !errors.Is(err, ops.ErrInvalid) {
		t.Errorf("List of an unknown kind = %v, want ErrInvalid", err)
	}
}

func TestList_SortedAndPerKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	if got, err := d.List(ctx, ops.KindSelection); err != nil || len(got) != 0 {
		t.Fatalf("List of nothing = %v, %v, want none", got, err)
	}
	var want []string
	for i := 0; i < 5; i++ {
		digest, err := d.Put(ctx, ops.KindSelection, []byte(fmt.Sprintf("program %d", i)))
		if err != nil {
			t.Fatal(err)
		}
		want = append(want, digest)
	}
	if _, err := d.Put(ctx, ops.KindSelectionCode, []byte("a code")); err != nil {
		t.Fatal(err)
	}
	got, err := d.List(ctx, ops.KindSelection)
	if err != nil || len(got) != 5 {
		t.Fatalf("List = %v, %v, want the five selections and not the code", got, err)
	}
	for i := 1; i < len(got); i++ {
		if got[i-1] >= got[i] {
			t.Fatalf("List is not sorted: %v", got)
		}
	}
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			t.Errorf("List misses %s", w)
		}
	}
}

// The authored tables are immutable for every writer: the triggers refuse a change
// and a removal. The checkpoint value table refuses a change, and a removal while a
// checkpoint row names the value.
func TestTriggers_RefuseWhatImmutableMeans(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	for _, tc := range []struct{ kind, update, remove string }{
		{ops.KindReceipt, `UPDATE receipt SET body = x'00' WHERE hash = ?`, `DELETE FROM receipt WHERE hash = ?`},
		{ops.KindServed, `UPDATE served_value SET body = x'00' WHERE hash = ?`, `DELETE FROM served_value WHERE hash = ?`},
		{ops.KindSelection, `UPDATE saved_selection SET body = x'00' WHERE hash = ?`, `DELETE FROM saved_selection WHERE hash = ?`},
		{ops.KindSelectionCode, `UPDATE saved_selection_code SET body = x'00' WHERE hash = ?`, `DELETE FROM saved_selection_code WHERE hash = ?`},
		{ops.KindCheckpoint, `UPDATE checkpoint_value SET body = x'00' WHERE hash = ?`, ""},
	} {
		digest, err := d.Put(ctx, tc.kind, []byte("held "+tc.kind))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.SQL().ExecContext(ctx, tc.update, digest); err == nil {
			t.Errorf("%s: an update was accepted", tc.kind)
		}
		if tc.remove == "" {
			continue
		}
		if _, err := d.SQL().ExecContext(ctx, tc.remove, digest); err == nil {
			t.Errorf("%s: a delete was accepted", tc.kind)
		}
	}

	// A checkpoint value goes only when no row names it.
	named, err := d.Put(ctx, ops.KindCheckpoint, []byte("named"))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.AppendCheckpoint(ctx, ops.CheckpointRow{SpanID: "bucket:2026", V: 1, CoversW: 1, ValueHash: named, MintedAt: "t"}); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL().ExecContext(ctx, `DELETE FROM checkpoint_value WHERE hash = ?`, named); err == nil {
		t.Error("a checkpoint value a row names was deleted")
	}
	unnamed, err := d.Put(ctx, ops.KindCheckpoint, []byte("unnamed"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.SQL().ExecContext(ctx, `DELETE FROM checkpoint_value WHERE hash = ?`, unnamed); err != nil {
		t.Errorf("a checkpoint value nothing names was refused: %v", err)
	}
}

// Two receipts of one read can Put the same bytes at once while a verify reads them. A
// reader sees nothing or a whole row, never a short one.
func TestPut_ConcurrentWritersAndReadersSeeWholeRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, _ := open(t)
	body := bytes.Repeat([]byte("whole "), 4096)
	digest := ops.Digest(body)

	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := d.Put(ctx, ops.KindReceipt, body); err != nil {
				errs <- fmt.Errorf("put: %w", err)
			}
		}()
		go func() {
			defer wg.Done()
			b, err := d.Get(ctx, ops.KindReceipt, digest)
			if err != nil && !errors.Is(err, ops.ErrNotFound) {
				errs <- fmt.Errorf("get: %w", err)
				return
			}
			if err == nil && !bytes.Equal(b, body) {
				errs <- fmt.Errorf("get: a short or altered row of %d bytes", len(b))
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got, err := d.List(ctx, ops.KindReceipt); err != nil || len(got) != 1 {
		t.Fatalf("List = %v, %v, want the one row", got, err)
	}
}

// A store written before the content tables is refused rather than read as one with
// no receipts: the files beside it hold them, and absent would read as none.
func TestOpen_RefusesTheSchemaBeforeContentRows(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	d, path := open(t)
	if _, err := d.SQL().ExecContext(ctx, `UPDATE meta SET value = '8' WHERE key = 'schema_version'`); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ops.Open(ctx, path, buckets(t)); !errors.Is(err, ops.ErrSchemaVersion) {
		t.Fatalf("Open of a version 8 store = %v, want ErrSchemaVersion", err)
	}
}

// The packages that file content name its kinds themselves, so that they need not
// import the store for a string. A name that drifted would be refused at the first
// write; this refuses it at the build.
func TestKinds_AgreeWithTheirOwners(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ owner, theirs, ours string }{
		{"receipt.Kind", receipt.Kind, ops.KindReceipt},
		{"receipt.ServedKind", receipt.ServedKind, ops.KindServed},
		{"selection.Kind", selection.Kind, ops.KindSelection},
		{"selection.CodeKind", selection.CodeKind, ops.KindSelectionCode},
	} {
		if tc.theirs != tc.ours {
			t.Errorf("%s = %q, the store names it %q", tc.owner, tc.theirs, tc.ours)
		}
	}
}
