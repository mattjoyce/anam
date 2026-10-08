package read

import (
	"context"
	"errors"
	"fmt"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// ChangeIndex is the operational store's index of anam-events, as far as it has
// replayed the ledger.
type ChangeIndex interface {
	LastSeq(ctx context.Context) (int64, error)
	ChangesSince(ctx context.Context, since, until string) ([]ops.EventRow, error)
}

// Changes reads the anam-events in the window q names from idx as it stands. It
// does not catch idx up: that is a write, and belongs to whoever may write. w is the
// ledger's head, so the answer says when the index is behind it.
func Changes(ctx context.Context, idx ChangeIndex, w int64, q api.ChangesQuery) (api.Changes, error) {
	loc, err := Location(q.Zone)
	if err != nil {
		return api.Changes{}, err
	}
	if q.Since == "" {
		return api.Changes{}, badQuery(errors.New("--since is required"))
	}
	sinceT, _, err := parseInstant(q.Since, loc)
	if err != nil {
		return api.Changes{}, badQuery(fmt.Errorf("--since: %w", err))
	}
	until := ""
	if q.Until != "" {
		t, dateOnly, err := parseInstant(q.Until, loc)
		if err != nil {
			return api.Changes{}, badQuery(fmt.Errorf("--until: %w", err))
		}
		if dateOnly {
			t = t.AddDate(0, 0, 1)
		}
		until = clock.Format(t)
	}

	seq, err := idx.LastSeq(ctx)
	if err != nil {
		return api.Changes{}, err
	}
	rows, err := idx.ChangesSince(ctx, clock.Format(sinceT), until)
	if err != nil {
		return api.Changes{}, err
	}
	c := api.Changes{
		IndexSeq: seq, W: w, Since: clock.Format(sinceT), Until: until,
		Changes: make([]api.Change, 0, len(rows)),
	}
	for _, r := range rows {
		c.Changes = append(c.Changes, api.Change{
			Seq: r.Seq, AnamTime: r.AnamTime, Kind: string(r.Kind), FacetID: r.FacetID,
			ArtifactVersion: r.ArtifactVersion,
		})
	}
	return c, nil
}
