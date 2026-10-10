package read

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// knowledge is which state of Anam's knowledge a read stands on: the head, the
// watermark an anam-time names (as_of), or a watermark named directly (w). At most one
// of asOf and w is set.
type knowledge struct {
	asOf time.Time
	w    ledger.Watermark
}

// knowledgeOf reads q's knowledge selector. as_of is a time Anam resolves to a W; w is
// the W itself, the direct address a receipt records, so a follow-up read can stand on
// the same knowledge as a cited one. Naming both is a contradiction, not a preference.
func knowledgeOf(q api.TimelineQuery, loc *time.Location) (knowledge, error) {
	if q.W < 0 {
		return knowledge{}, badQuery(fmt.Errorf("w %d: a watermark is not negative", q.W))
	}
	if q.W > 0 && q.AsOf != "" {
		return knowledge{}, badQuery(errors.New("as_of and w both choose what Anam knew: give one"))
	}
	asOf, err := parseAsOf(q.AsOf, loc)
	if err != nil {
		return knowledge{}, badQuery(err)
	}
	return knowledge{asOf: asOf, w: ledger.Watermark(q.W)}, nil
}

// watermarkOf is the head, the watermark an anam-time names, or the one named. A
// watermark above the head is refused: it names knowledge Anam does not have yet.
func watermarkOf(ctx context.Context, l Ledger, know knowledge, v *view) (ledger.Watermark, error) {
	if know.w > 0 {
		head, err := l.Head(ctx)
		if err != nil {
			return 0, err
		}
		if know.w > head {
			return 0, badQuery(fmt.Errorf("w %d is above the head, %d", know.w, head))
		}
		return know.w, nil
	}
	if know.asOf.IsZero() {
		return l.Head(ctx)
	}
	v.asOfAt = clock.Format(know.asOf)
	return fold.WatermarkAt(ctx, l, know.asOf)
}
