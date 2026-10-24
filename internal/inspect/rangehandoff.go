package inspect

import (
	"net/url"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
)

// The hand-off between the timeline and the select page (card 237). Each is a link to
// an existing route with the parameters that route already reads, so there is no new
// state: the window, the type filter and the zone ride in the address. A link carries
// only what both ends read, so not the source (the timeline has none) and not the
// status. Both ends read a bare date as a day in the zone the link names, so a Sydney
// day written by one is the same Sydney day read by the other.

// selectRangeLink is the select page over the timeline's window: from and to as dates
// when the window lies on midnights in the zone, else as instants, and the types the
// band shows (none named means every type). The zone travels with them.
func selectRangeLink(from, to time.Time, types []facet.Type, z readZone) string {
	q := url.Values{}
	q.Set("from", lexemeIn(from, z.Loc, false))
	q.Set("to", lexemeIn(to, z.Loc, true))
	q.Set("zone", z.Name)
	for _, t := range types {
		q.Add("type", string(t))
	}
	return address("/select", q)
}

// timelineLink is the timeline over the select page's filter as it stands: its from
// and to when it has them, its types, the zone when a date is written, and the saved
// selection drawn as it is today when there is one (code). held is every type the
// record holds, so a type prefix names the types it covers: the timeline filters by
// exact type. A filter with nothing to say opens the whole person, named outright
// since a bare "/" may open the default view.
func (st selectState) timelineLink(held []string, code string) string {
	q := url.Values{}
	f := st.filter
	if f.From != "" {
		if t, err := clock.Parse(f.From); err == nil {
			q.Set("from", boundIn(t, st.zone.Loc, false))
		}
	}
	if f.To != "" {
		if t, err := clock.Parse(f.To); err == nil {
			q.Set("to", boundIn(t, st.zone.Loc, true))
		}
	}
	if q.Get("from") != "" || q.Get("to") != "" {
		q.Set("zone", st.zone.Name)
	}
	if len(f.Types) > 0 {
		var covered []string
		for _, t := range held {
			for _, p := range f.Types {
				if facet.Matches(facet.Type(t), p) {
					covered = append(covered, t)
					break
				}
			}
		}
		if len(covered) == 0 {
			covered = []string{typeNone} // the filter covers nothing the record holds
		}
		q["type"] = covered
	}
	if code != "" {
		q.Set("sel", code)
	}
	if len(q) == 0 {
		q.Set("view", viewWhole)
	}
	return address("/", q)
}
