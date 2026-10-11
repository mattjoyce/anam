package inspect

import (
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

// A timeline view is a URL: the window, the type filter and the status filter ride
// in the query, so every move is a link and a view can be shared or reloaded as it
// stands.

// statusFilter narrows the band by folded status (card 131). The band is the ledger
// surface and draws everything by default; a retired source leaves thousands of
// tombstones under the live record, and this is the lens that hides them, or shows
// them alone.
type statusFilter string

const (
	statusAll  statusFilter = ""     // every facet, the dead faded
	statusLive statusFilter = "live" // the live alone: what the canonical composes
	statusDead statusFilter = "dead" // the tombstoned and rejected alone
)

// parseStatus reads the status query; an unknown value is the caller's error.
func parseStatus(q string) (statusFilter, error) {
	switch s := statusFilter(q); s {
	case statusAll, statusLive, statusDead:
		return s, nil
	default:
		return "", fmt.Errorf("status: %q is not live, dead or empty", q)
	}
}

// admits says whether a facet in the given liveness is drawn under the filter.
func (s statusFilter) admits(live bool) bool {
	switch s {
	case statusLive:
		return live
	case statusDead:
		return !live
	default:
		return true
	}
}

// tlView is what every timeline link carries besides the window: the type filter
// (nil for all) and the status filter.
type tlView struct {
	Types  []facet.Type
	Status statusFilter
}

func (v tlView) addTo(q url.Values) {
	for _, t := range v.Types {
		q.Add("type", string(t))
	}
	if v.Status != statusAll {
		q.Set("status", string(v.Status))
	}
}

// navLinks derives the four moves from the window: pan by a quarter of its width,
// zoom in to half and out to double, both centred. The window and the filters stay
// in the URL, so a view is a link.
func navLinks(from, to time.Time, view tlView) tlNav {
	width := to.Sub(from)
	quarter := width / 4
	centre := from.Add(width / 2)
	link := func(f, t time.Time) string {
		q := url.Values{}
		q.Set("from", f.UTC().Format(time.RFC3339))
		q.Set("to", t.UTC().Format(time.RFC3339))
		view.addTo(q)
		return "/?" + q.Encode()
	}
	return tlNav{
		Left:  link(from.Add(-quarter), to.Add(-quarter)),
		Right: link(from.Add(quarter), to.Add(quarter)),
		In:    link(centre.Add(-width/4), centre.Add(width/4)),
		Out:   link(centre.Add(-width), centre.Add(width)),
	}
}

// typeFilter reads the type query: no type named shows every type. A name the
// ledger does not hold is dropped, so a stale link narrows to what still exists.
// It returns the set to draw and the selection to carry on links (nil for all).
func typeFilter(typeQ []string, types []facet.Type) (show map[facet.Type]bool, selected []facet.Type) {
	show = map[facet.Type]bool{}
	held := map[facet.Type]bool{}
	for _, t := range types {
		held[t] = true
	}
	for _, q := range typeQ {
		if t := facet.Type(q); held[t] && !show[t] {
			show[t] = true
			selected = append(selected, t)
		}
	}
	if len(selected) == 0 || len(selected) == len(types) {
		for _, t := range types {
			show[t] = true
		}
		return show, nil
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i] < selected[j] })
	return show, selected
}

// windowLink is the current window under a different view.
func windowLink(from, to time.Time, zoomed bool, view tlView) string {
	q := url.Values{}
	if zoomed {
		q.Set("from", from.UTC().Format(time.RFC3339))
		q.Set("to", to.UTC().Format(time.RFC3339))
	}
	view.addTo(q)
	if len(q) == 0 {
		return "/"
	}
	return "/?" + q.Encode()
}
