package inspect

import (
	"context"
	"fmt"
	"sort"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Checkpoints is the narrow read surface of the checkpoint log the inspector needs
// (interfaces at the point of use; the daemon's read-only *ops.DB satisfies it). Nil
// means the inspector has no log to show and the timeline draws no bucket strip.
type Checkpoints interface {
	SpanHeads(ctx context.Context) (map[string]ops.CheckpointRow, error)
	LastMintRun(ctx context.Context, w int64) (run int64, ok bool, err error)
	LogSealed(ctx context.Context) (bool, error)
}

// The three states a bucket can be in, each one hash comparison (card 21, the
// hard-and-soft-checkpoints decision of 2026-10-06): the bucket's live value at W,
// computed for this page and dropped, against the head checkpoint the log holds.
const (
	bucketSealed = "sealed" // the head's hash equals the value at W
	bucketBehind = "behind" // the value at W differs: a correct floor that costs replay, never wrong
	bucketNever  = "never"  // live facets fall in the bucket and the log has no head for it
)

// bucketState is one leaf bucket of the checkpoint layout with its state at W.
type bucketState struct {
	Bucket span.Bucket
	State  string
	Head   ops.CheckpointRow // zero when never minted
	Facets int               // live facets in the bucket at W
}

// checkpointStates compares every bucket's value at W with the log's heads. The
// leaves come from the log itself (span.LayoutOf over the head ids), since the mint
// subdivides buckets over its budget, so the strip shows the buckets the mint
// actually keeps; a bucket with live facets and no head is never minted. Nothing is
// kept between requests: a soft checkpoint is computed, compared and dropped.
func checkpointStates(ctx context.Context, cp Checkpoints, res fold.Result, fallback span.Buckets) ([]bucketState, tlCheckpoints, error) {
	heads, err := cp.SpanHeads(ctx)
	if err != nil {
		return nil, tlCheckpoints{}, err
	}
	var placer mint.Placer = fallback
	var layout span.Layout
	if len(heads) > 0 {
		ids := make([]string, 0, len(heads))
		for id := range heads {
			ids = append(ids, id)
		}
		if layout, err = span.LayoutOf(ids); err != nil {
			return nil, tlCheckpoints{}, fmt.Errorf("inspect: checkpoint layout: %w", err)
		}
		placer = layout
	}
	parts, err := mint.Partition(res.Value, placer)
	if err != nil {
		return nil, tlCheckpoints{}, err
	}

	states := map[string]bucketState{}
	for id, head := range heads {
		b, err := span.Parse(id)
		if err != nil {
			return nil, tlCheckpoints{}, fmt.Errorf("inspect: checkpoint %s: %w", id, err)
		}
		if !layout.Leaf(b) {
			continue // split by the mint; its children are the leaves
		}
		soft, err := fold.NewValue(parts[id])
		if err != nil {
			return nil, tlCheckpoints{}, err
		}
		st := bucketBehind
		if soft.Hash() == head.ValueHash {
			st = bucketSealed
		}
		states[id] = bucketState{Bucket: b, State: st, Head: head, Facets: len(parts[id])}
	}
	for id, entries := range parts {
		if _, ok := states[id]; ok {
			continue
		}
		b, err := span.Parse(id)
		if err != nil {
			return nil, tlCheckpoints{}, fmt.Errorf("inspect: bucket %s: %w", id, err)
		}
		states[id] = bucketState{Bucket: b, State: bucketNever, Facets: len(entries)}
	}

	out := make([]bucketState, 0, len(states))
	summary := tlCheckpoints{Have: true}
	for _, st := range states {
		out = append(out, st)
		summary.Total++
		switch st.State {
		case bucketSealed:
			summary.Sealed++
		case bucketBehind:
			summary.Behind++
		default:
			summary.Never++
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Bucket.From().Before(out[j].Bucket.From()) })

	if summary.Run, summary.HasRun, err = cp.LastMintRun(ctx, int64(res.W)); err != nil {
		return nil, tlCheckpoints{}, err
	}
	if summary.LogSealed, err = cp.LogSealed(ctx); err != nil {
		return nil, tlCheckpoints{}, err
	}
	return out, summary, nil
}

// bucketTitle is the strip's hover text for one bucket: the id, its state and the
// head it was compared with, in the words the glossary uses.
func bucketTitle(st bucketState, w int64) string {
	facets := fmt.Sprintf("%d live facet%s", st.Facets, plural(st.Facets))
	switch st.State {
	case bucketSealed:
		return fmt.Sprintf("%s · sealed · v%d covers W=%d · %s", st.Bucket.ID(), st.Head.V, st.Head.CoversW, facets)
	case bucketBehind:
		return fmt.Sprintf("%s · behind: the value at W=%d differs from v%d (covers W=%d) · %s · click to zoom in",
			st.Bucket.ID(), w, st.Head.V, st.Head.CoversW, facets)
	default:
		return fmt.Sprintf("%s · never minted · %s · click to zoom in", st.Bucket.ID(), facets)
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
