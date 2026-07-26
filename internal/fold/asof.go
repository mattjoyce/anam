package fold

import (
	"context"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// WatermarkAt translates an anam-time into the watermark in force at that instant:
// the greatest seq whose anam-event happened at or before t.
//
// This is the bridge between the two clocks (§5). A person asks "what did the bp view
// show me last Tuesday?" in anam-time, because that is the clock they experienced;
// every read below this line is in watermarks, because that is the clock the ledger
// orders by. Nothing else in Anam should be converting between them ad hoc.
//
// A t before the first anam-event yields 0 — the empty person, which ReadAt answers
// honestly rather than treating as an error. A t after the last yields the head.
//
// It replays to find the boundary rather than asking the operational store, so an
// as-of read needs no derived tier and no catch-up: the ledger alone can answer a
// question about itself. The caller then hands the watermark to ReadAt, which replays
// again — two passes over the same events, which at this scale is not worth a cleverer
// arrangement that would couple the two.
func WatermarkAt(ctx context.Context, l Ledger, t time.Time) (ledger.Watermark, error) {
	head, err := l.Head(ctx)
	if err != nil {
		return 0, err
	}
	events, err := l.Replay(ctx, 0, head)
	if err != nil {
		return 0, err
	}

	// Anam-times are fixed-width clock.Layout strings and seq is monotonic in
	// anam-time, so this is a scan for the last event at or before the cut.
	cut := clock.Format(t)
	var w ledger.Watermark
	for _, ev := range events {
		if clock.Format(ev.AnamTime) > cut {
			break
		}
		w = ledger.Watermark(ev.Seq)
	}
	return w, nil
}
