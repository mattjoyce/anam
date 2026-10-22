package ops_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ops"
)

// Collection of old checkpoints (card 215), over a log built by hand as the mint
// builds one: rows and values first, then the run that accounts for them.

// logBuilder writes checkpoint rows and values and records runs, as a mint does.
type logBuilder struct {
	t   *testing.T
	d   *ops.DB
	ctx context.Context
}

func newLog(t *testing.T) *logBuilder {
	t.Helper()
	d, _ := open(t)
	return &logBuilder{t: t, d: d, ctx: context.Background()}
}

// row files a checkpoint of span at v, covering w, holding the value named by label;
// the same label is the same value.
func (b *logBuilder) row(span string, v int, w int64, label string) string {
	b.t.Helper()
	digest, err := b.d.Put(b.ctx, ops.KindCheckpoint, []byte("value "+label))
	if err != nil {
		b.t.Fatal(err)
	}
	if err := b.d.AppendCheckpoint(b.ctx, ops.CheckpointRow{SpanID: span, V: v, CoversW: w, ValueHash: digest, MintedAt: "t"}); err != nil {
		b.t.Fatal(err)
	}
	if err := b.d.AppendCheckpointSize(b.ctx, span, v, 1); err != nil {
		b.t.Fatal(err)
	}
	return digest
}

func (b *logBuilder) run(w int64) {
	b.t.Helper()
	if err := b.d.AppendMintRun(b.ctx, w, "t"); err != nil {
		b.t.Fatal(err)
	}
}

// receipt files a receipt whose watermark is w.
func (b *logBuilder) receipt(w int) {
	b.t.Helper()
	if _, err := b.d.Put(b.ctx, ops.KindReceipt, []byte(fmt.Sprintf(`{"watermark":%d,"served_hash":"sha256:x"}`, w))); err != nil {
		b.t.Fatal(err)
	}
}

// at is what a read from the run at w takes, as "span vN" in span order.
func (b *logBuilder) at(w int64) []string {
	b.t.Helper()
	rows, err := b.d.CheckpointsAt(b.ctx, w)
	if err != nil {
		b.t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s v%d", r.SpanID, r.V))
	}
	return out
}

func (b *logBuilder) runs() []int64 {
	b.t.Helper()
	var out []int64
	for w := int64(0); w <= 100; w++ {
		if run, ok, err := b.d.LastMintRun(b.ctx, w); err != nil {
			b.t.Fatal(err)
		} else if ok && run == w {
			out = append(out, w)
		}
	}
	return out
}

func (b *logBuilder) collect() ops.Collected {
	b.t.Helper()
	got, err := b.d.CollectCheckpoints(b.ctx)
	if err != nil {
		b.t.Fatalf("CollectCheckpoints: %v", err)
	}
	return got
}

func same[T comparable](t *testing.T, what string, got, want []T) {
	t.Helper()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}

// buildThreeRuns is two spans minted over three runs: A at every run, B at the first and
// the last.
func buildThreeRuns(b *logBuilder) {
	b.row("A", 1, 10, "a1")
	b.row("B", 1, 10, "b1")
	b.run(10)
	b.row("A", 2, 20, "a2")
	b.run(20)
	b.row("A", 3, 30, "a3")
	b.row("B", 2, 30, "b2")
	b.run(30)
}

func TestCollect_KeepsTheNewestRunAndTheHeadsAndNothingElse(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	buildThreeRuns(b)

	got := b.collect()
	want := ops.Collected{Runs: 2, Checkpoints: 3, Values: 3}
	if got.Runs != want.Runs || got.Checkpoints != want.Checkpoints || got.Values != want.Values || got.Bytes == 0 {
		t.Errorf("collected %+v, want %d runs, %d checkpoints, %d values and their bytes", got, want.Runs, want.Checkpoints, want.Values)
	}
	same(t, "the log", func() []string {
		rows, _ := b.d.Checkpoints(b.ctx)
		var out []string
		for _, r := range rows {
			out = append(out, fmt.Sprintf("%s v%d", r.SpanID, r.V))
		}
		return out
	}(), []string{"A v3", "B v2"})
	same(t, "the runs", b.runs(), []int64{30})
	same(t, "a read at the newest run", b.at(30), []string{"A v3", "B v2"})
	// A watermark between the runs removed has no run left to start from.
	if _, ok, _ := b.d.LastMintRun(b.ctx, 25); ok {
		t.Error("a read at 25 still finds a run")
	}
	digests, _ := b.d.List(b.ctx, ops.KindCheckpoint)
	if len(digests) != 2 {
		t.Errorf("%d checkpoint values remain, want the 2 the heads name", len(digests))
	}
	sizes, _ := b.d.CheckpointSizes(b.ctx)
	if len(sizes) != 2 {
		t.Errorf("%d sizes remain, want 2", len(sizes))
	}
	if sealed, err := b.d.LogSealed(b.ctx); err != nil || !sealed {
		t.Errorf("LogSealed after a collection = %v, %v, want sealed at the rows that remain", sealed, err)
	}
	if again := b.collect(); !again.None() {
		t.Errorf("a second collection removed %+v", again)
	}
}

// A receipt pins the run its watermark starts from, and with it the checkpoints that run
// reads: verifying it later still starts from a floor.
func TestCollect_AReceiptPinsTheRunItReadsFrom(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	buildThreeRuns(b)
	b.receipt(25) // starts from the run at 20

	got := b.collect()
	if got.Runs != 1 || got.Checkpoints != 1 {
		t.Errorf("collected %+v, want the run at 10 and A v1", got)
	}
	same(t, "the runs", b.runs(), []int64{20, 30})
	same(t, "a read from the pinned run", b.at(20), []string{"A v2", "B v1"})
	same(t, "a read from the newest run", b.at(30), []string{"A v3", "B v2"})
	if sealed, _ := b.d.LogSealed(b.ctx); !sealed {
		t.Error("the log is not sealed after a collection")
	}
}

func TestCollect_AReceiptThatPinsNothing(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	buildThreeRuns(b)
	b.receipt(3) // below every run
	for _, body := range []string{`not json`, `{"served_hash":"sha256:x"}`, `{"watermark":"soon"}`} {
		if _, err := b.d.Put(b.ctx, ops.KindReceipt, []byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	got := b.collect()
	if got.Runs != 2 || got.Checkpoints != 3 {
		t.Errorf("collected %+v, want the same as with no receipt: 2 runs and 3 checkpoints", got)
	}
}

// A value two versions share goes only when no row names it: A returns to the value it
// had.
func TestCollect_KeepsAValueARowStillNames(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	b.row("A", 1, 10, "same")
	b.run(10)
	b.row("A", 2, 20, "other")
	b.run(20)
	b.row("A", 3, 30, "same")
	b.run(30)

	got := b.collect()
	if got.Checkpoints != 2 || got.Values != 1 {
		t.Errorf("collected %+v, want 2 checkpoints and the one value only they named", got)
	}
	rows, _ := b.d.Checkpoints(b.ctx)
	if len(rows) != 1 {
		t.Fatalf("the log holds %d rows, want the head", len(rows))
	}
	if _, err := b.d.Get(b.ctx, ops.KindCheckpoint, rows[0].ValueHash); err != nil {
		t.Errorf("the head's value is gone: %v", err)
	}
}

// A log holding rows from a mint that crashed is not sealed, and collecting around them
// would seal them in as a run's own. It waits for the next run.
func TestCollect_WaitsForASealedLog(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	buildThreeRuns(b)
	b.row("A", 4, 40, "a4") // a mint that crashed after this row, before its run
	if got := b.collect(); !got.None() {
		t.Fatalf("collected %+v from an unsealed log", got)
	}
	if sealed, _ := b.d.LogSealed(b.ctx); sealed {
		t.Fatal("the collection sealed an unsealed log")
	}
	b.run(40)
	if got := b.collect(); got.Checkpoints != 4 || got.Runs != 3 {
		t.Errorf("after the run, collected %+v, want 4 checkpoints and 3 runs", got)
	}
}

func TestCollect_AnEmptyLog(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	if got := b.collect(); !got.None() {
		t.Errorf("collected %+v from an empty log", got)
	}
}

// A value put by a move that no row names is as unread as one a collected row named, so
// the next collection takes it.
func TestCollect_RemovesValuesNoRowNames(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	b.row("A", 1, 10, "a1")
	b.run(10)
	orphan, err := b.d.Put(b.ctx, ops.KindCheckpoint, []byte("an orphan"))
	if err != nil {
		t.Fatal(err)
	}
	if got := b.collect(); got.Values != 1 {
		t.Errorf("collected %+v, want the orphan", got)
	}
	if _, err := b.d.Size(b.ctx, ops.KindCheckpoint, orphan); err == nil {
		t.Error("the orphan survived")
	}
}
