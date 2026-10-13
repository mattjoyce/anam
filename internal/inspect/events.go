package inspect

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The events page: the ledger's anam-events, newest first, a page at a time.

type eventsData struct {
	W      ledger.Watermark
	Events []eventRow
	// Older is the link to the page before this one; empty on the first events.
	Older string
}

type eventRow struct {
	facet.AnamEvent
	AnamTimeText string
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	head, err := s.ledger.Head(ctx)
	if err != nil {
		httpError(w, err)
		return
	}
	var before int64
	if q := r.URL.Query().Get("before"); q != "" {
		n, err := strconv.ParseInt(q, 10, 64)
		if err != nil || n < 1 {
			http.Error(w, fmt.Sprintf("?before=%q: want a positive event seq", q), http.StatusBadRequest)
			return
		}
		before = n
	}
	since, upto := eventPage(head, before)
	events, err := s.ledger.Replay(ctx, since, upto)
	if err != nil {
		httpError(w, err)
		return
	}
	events = newestFirst(events)
	rows := make([]eventRow, len(events))
	for i, ev := range events {
		rows[i] = eventRow{AnamEvent: ev, AnamTimeText: ev.AnamTime.Format(time.RFC3339)}
	}
	data := eventsData{W: head, Events: rows}
	if since > 0 {
		data.Older = "/events?before=" + strconv.FormatInt(int64(since)+1, 10)
	}
	render(w, "events.html", data)
}

// headAndEvents reads the watermark and the full anam-event stream — the inspector
// re-reads rather than caches, so what it shows is what the ledger holds right now.
func (s *Server) headAndEvents(ctx context.Context) (ledger.Watermark, []facet.AnamEvent, error) {
	head, err := s.ledger.Head(ctx)
	if err != nil {
		return 0, nil, err
	}
	events, err := s.ledger.Replay(ctx, 0, head)
	if err != nil {
		return 0, nil, err
	}
	return head, events, nil
}
