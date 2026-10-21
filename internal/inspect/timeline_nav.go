package inspect

import (
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

// A timeline view is a URL: the window, the type filter and the status filter ride
// in the query, so every move is a link and a view can be shared, reloaded as it
// stands, or kept as the default (card 135).

// statusFilter narrows the band by folded status (card 131). The band draws the
// live by default (card 135, Matt's call): a retired source leaves thousands of
// tombstones under the live record and every re-versioned hour of positions leaves
// another, so the dead are read from the ledger only when asked for, alone or
// beside the live.
type statusFilter string

const (
	statusLive statusFilter = ""     // the live alone: what the canonical composes
	statusAll  statusFilter = "all"  // every facet, the dead faded
	statusDead statusFilter = "dead" // the tombstoned and rejected alone
)

// parseStatus reads the status query; an unknown value is the caller's error. The
// default has two spellings, none and "live", so an old link still reads.
func parseStatus(q string) (statusFilter, error) {
	switch s := statusFilter(q); s {
	case statusLive, statusAll, statusDead:
		return s, nil
	case "live":
		return statusLive, nil
	default:
		return "", fmt.Errorf("status: %q is not all, dead or live", q)
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

// wantsDead says whether the band needs the dead facets' versions at all.
func (s statusFilter) wantsDead() bool { return s != statusLive }

// typeNone is the type query that unchecks every type, the partner of "all types".
// No facet type can collide with it: types are dotted and namespaced.
const typeNone = "none"

// viewWhole is the query that names the whole person outright, for when a bare "/"
// opens the default view instead.
const viewWhole = "whole"

// tlView is what every timeline link carries besides the window: the type filter
// (nil for all, None for none) and the status filter.
type tlView struct {
	Types  []facet.Type
	None   bool
	Status statusFilter
	// Sel is the saved selection drawn (card 206), the zero value for none.
	Sel selRef
}

// selRef is a saved selection as a timeline link carries it: its code and how it is
// drawn.
type selRef struct{ Code, Mode string }

func (v tlView) addTo(q url.Values) {
	if v.Sel.Code != "" {
		q.Set("sel", v.Sel.Code)
		if v.Sel.Mode == selHighlight {
			q.Set("selmode", selHighlight)
		}
	}
	for _, t := range v.Types {
		q.Add("type", string(t))
	}
	if v.None && len(v.Types) == 0 {
		q.Set("type", typeNone)
	}
	if v.Status != statusLive {
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
		return spanLink(f, t, view)
	}
	return tlNav{
		Left:  link(from.Add(-quarter), to.Add(-quarter)),
		Right: link(from.Add(quarter), to.Add(quarter)),
		In:    link(centre.Add(-width/4), centre.Add(width/4)),
		Out:   link(centre.Add(-width), centre.Add(width)),
	}
}

// spanLink is the view over an explicit window.
func spanLink(from, to time.Time, view tlView) string {
	q := url.Values{}
	q.Set("from", from.UTC().Format(time.RFC3339))
	q.Set("to", to.UTC().Format(time.RFC3339))
	view.addTo(q)
	return "/?" + q.Encode()
}

// typeFilter reads the type query: no type named shows every type, and "none"
// alone shows none. A name the ledger does not hold is dropped, so a stale link
// narrows to what still exists. It returns the set to draw and the selection to
// carry on links (nil for all).
func typeFilter(typeQ []string, types []facet.Type) (show map[facet.Type]bool, selected []facet.Type, none bool) {
	show = map[facet.Type]bool{}
	held := map[facet.Type]bool{}
	for _, t := range types {
		held[t] = true
	}
	for _, q := range typeQ {
		if q == typeNone {
			none = true
			continue
		}
		if t := facet.Type(q); held[t] && !show[t] {
			show[t] = true
			selected = append(selected, t)
		}
	}
	if len(selected) == 0 && none {
		return show, nil, true
	}
	if len(selected) == 0 || len(selected) == len(types) {
		for _, t := range types {
			show[t] = true
		}
		return show, nil, false
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i] < selected[j] })
	return show, selected, false
}

// windowLink is the current window under a different view. The whole person with
// nothing else to say is named outright, since a bare "/" may open the default
// view instead.
func windowLink(from, to time.Time, zoomed bool, view tlView) string {
	q := url.Values{}
	if zoomed {
		q.Set("from", from.UTC().Format(time.RFC3339))
		q.Set("to", to.UTC().Format(time.RFC3339))
	}
	view.addTo(q)
	if len(q) == 0 {
		q.Set("view", viewWhole)
	}
	return "/?" + q.Encode()
}
