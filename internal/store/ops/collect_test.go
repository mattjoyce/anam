package ops_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
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

func (b *logBuilder) collect() {
	b.t.Helper()
	if err := b.d.CollectCheckpoints(b.ctx); err != nil {
		b.t.Fatalf("CollectCheckpoints: %v", err)
	}
}

// log is the rows the log holds, as "span vN" in span order.
func (b *logBuilder) log() []string {
	b.t.Helper()
	rows, err := b.d.Checkpoints(b.ctx)
	if err != nil {
		b.t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		out = append(out, fmt.Sprintf("%s v%d", r.SpanID, r.V))
	}
	return out
}

func (b *logBuilder) values() int {
	b.t.Helper()
	digests, err := b.d.List(b.ctx, ops.KindCheckpoint)
	if err != nil {
		b.t.Fatal(err)
	}
	return len(digests)
}

// readable reports whether a read from the run at w finds its checkpoints all there.
func (b *logBuilder) readable(w int64) bool {
	b.t.Helper()
	_, ok, err := b.d.RunCheckpoints(b.ctx, w)
	if err != nil {
		b.t.Fatal(err)
	}
	return ok
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

// A collection keeps what the newest run reads, which is each span's head, and nothing
// else: not the older versions, their sizes or their values.
func TestCollect_KeepsWhatTheNewestRunReads(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	buildThreeRuns(b)
	if !b.readable(10) || !b.readable(20) || !b.readable(30) {
		t.Fatal("a run is not readable before any collection")
	}

	b.collect()
	same(t, "the log", b.log(), []string{"A v3", "B v2"})
	same(t, "a read from the newest run", b.at(30), []string{"A v3", "B v2"})
	if n := b.values(); n != 2 {
		t.Errorf("%d checkpoint values remain, want the 2 the heads name", n)
	}
	if sizes, _ := b.d.CheckpointSizes(b.ctx); len(sizes) != 2 {
		t.Errorf("%d sizes remain, want 2", len(sizes))
	}
	if sealed, err := b.d.LogSealed(b.ctx); err != nil || !sealed {
		t.Errorf("LogSealed after a collection = %v, %v, want sealed", sealed, err)
	}
	if !b.readable(30) {
		t.Error("the newest run cannot be read after a collection")
	}
	// The older runs saw checkpoints that are gone, so they decline.
	if b.readable(10) || b.readable(20) {
		t.Error("an older run whose checkpoints were removed still reads")
	}
	b.collect()
	same(t, "the log after a second collection", b.log(), []string{"A v3", "B v2"})
}

// A run whose checkpoints are still the newest run's keeps reading: nothing it needs
// is gone, so there is nothing for it to decline over.
func TestCollect_AnOlderRunWhoseCheckpointsStandStillReads(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	b.row("A", 1, 10, "a1")
	b.run(10)
	b.row("B", 1, 20, "b1")
	b.run(20)
	b.run(30) // nothing new: the same checkpoints as the run at 20
	b.collect()
	if !b.readable(30) || !b.readable(20) || !b.readable(10) {
		t.Error("runs whose checkpoints all stand stopped reading")
	}
	b.row("A", 2, 40, "a2")
	b.run(40)
	b.collect()
	if !b.readable(40) || b.readable(30) || b.readable(10) {
		t.Error("after A moved on, only the newest run should read: the others saw A v1")
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
	b.collect()
	same(t, "the log", b.log(), []string{"A v3"})
	if n := b.values(); n != 1 {
		t.Fatalf("%d values remain, want the head's", n)
	}
	rows, _ := b.d.Checkpoints(b.ctx)
	if _, err := b.d.Get(b.ctx, ops.KindCheckpoint, rows[0].ValueHash); err != nil {
		t.Errorf("the head's value is gone: %v", err)
	}
}

// A row written after the newest run, by a mint that crashed before it recorded its run,
// is not read and goes; the log is as the run left it.
func TestCollect_RemovesWhatACrashedMintWroteAfterTheRun(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	buildThreeRuns(b)
	b.row("A", 4, 40, "a4")
	b.row("C", 1, 40, "c1")
	if sealed, _ := b.d.LogSealed(b.ctx); sealed {
		t.Fatal("a log holding rows after its newest run reads as sealed")
	}
	b.collect()
	same(t, "the log", b.log(), []string{"A v3", "B v2"})
	if sealed, _ := b.d.LogSealed(b.ctx); !sealed {
		t.Error("the log is not sealed once the rows no run accounts for are gone")
	}
}

// With no run recorded nothing is read, and nothing is removed.
func TestCollect_NoRunRemovesNothing(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	b.row("A", 1, 10, "a1")
	b.row("A", 2, 20, "a2")
	b.collect()
	same(t, "the log", b.log(), []string{"A v1", "A v2"})
	if n := b.values(); n != 2 {
		t.Errorf("%d values remain, want 2", n)
	}
}

// A value put by a move that no row names is as unread as one a collected row named.
func TestCollect_RemovesValuesNoRowNames(t *testing.T) {
	t.Parallel()
	b := newLog(t)
	b.row("A", 1, 10, "a1")
	b.run(10)
	orphan, err := b.d.Put(b.ctx, ops.KindCheckpoint, []byte("an orphan"))
	if err != nil {
		t.Fatal(err)
	}
	b.collect()
	if _, err := b.d.Size(b.ctx, ops.KindCheckpoint, orphan); err == nil {
		t.Error("the orphan survived")
	}
	if n := b.values(); n != 1 {
		t.Errorf("%d values remain, want the head's", n)
	}
}

// A read from a run takes the checkpoints the run saw. A log that lost one, replaced
// one, or gained one since does not yield them, and the read must be told so rather
// than composed from what is left: that is the one way a cache that lost entries could
// give a value with a bucket missing or stale in it.
func TestRunCheckpoints_NoticesWhatIsGoneChangedOrAdded(t *testing.T) {
	t.Parallel()
	build := func(t *testing.T) *logBuilder {
		b := newLog(t)
		b.row("A", 1, 10, "a1")
		b.row("B", 1, 10, "b1")
		b.run(10)
		b.row("A", 2, 20, "a2")
		b.run(20)
		return b
	}
	for _, tc := range []struct {
		name string
		hurt func(t *testing.T, b *logBuilder)
	}{
		{"nothing hurt", func(*testing.T, *logBuilder) {}},
		{"a span's only row lost", func(t *testing.T, b *logBuilder) {
			if err := opstest.LoseCheckpointRow(b.ctx, b.d, "B", 1); err != nil {
				t.Fatal(err)
			}
		}},
		{"the head lost, leaving the version before it", func(t *testing.T, b *logBuilder) {
			if err := opstest.LoseCheckpointRow(b.ctx, b.d, "A", 2); err != nil {
				t.Fatal(err)
			}
		}},
		{"a row added at a version the run did not see", func(t *testing.T, b *logBuilder) {
			b.row("A", 3, 20, "a3") // same covers_w as the run: it would now be the pick
		}},
		{"a span added at or below the run", func(t *testing.T, b *logBuilder) {
			b.row("C", 1, 15, "c1")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := build(t)
			if !b.readable(20) || !b.readable(10) {
				t.Fatal("the runs do not read before anything is hurt")
			}
			tc.hurt(t, b)
			want := tc.name == "nothing hurt"
			if got := b.readable(20); got != want {
				t.Errorf("readable(20) = %v, want %v", got, want)
			}
		})
	}
	b := build(t)
	if _, ok, err := b.d.RunCheckpoints(b.ctx, 15); err != nil || ok {
		t.Errorf("a w that is no run reads: ok=%v err=%v", ok, err)
	}
	// An older run whose pick for a span was lost, though a newer version remains: the
	// plain query would answer with no row for the span at all.
	if err := opstest.LoseCheckpointRow(b.ctx, b.d, "A", 1); err != nil {
		t.Fatal(err)
	}
	same(t, "the plain query at 10", b.at(10), []string{"B v1"})
	if b.readable(10) {
		t.Error("RunCheckpoints at 10 read a log with A missing")
	}
}
