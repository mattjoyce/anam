package inspect

import (
	"net/url"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

// A timeline view is a URL: the window and the type filter ride in the query, so
// every move is a link and a view can be shared or reloaded as it stands.

// navLinks derives the four moves from the window: pan by a quarter of its width,
// zoom in to half and out to double, both centred. The window and the type filter
// stay in the URL, so a view is a link.
func navLinks(from, to time.Time, types []facet.Type) tlNav {
	width := to.Sub(from)
	quarter := width / 4
	centre := from.Add(width / 2)
	link := func(f, t time.Time) string {
		q := url.Values{}
		q.Set("from", f.UTC().Format(time.RFC3339))
		q.Set("to", t.UTC().Format(time.RFC3339))
		for _, typ := range types {
			q.Add("type", string(typ))
		}
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

// windowLink is the current window with a different type filter.
func windowLink(from, to time.Time, zoomed bool, types []facet.Type) string {
	q := url.Values{}
	if zoomed {
		q.Set("from", from.UTC().Format(time.RFC3339))
		q.Set("to", to.UTC().Format(time.RFC3339))
	}
	for _, t := range types {
		q.Add("type", string(t))
	}
	if len(q) == 0 {
		return "/"
	}
	return "/?" + q.Encode()
}
