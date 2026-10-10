package inspect

import (
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The two list pages are bounded (card 85): at the live record each would otherwise
// be megabytes of rows that answer no question. The facet list filters by type,
// status and source and pages newest first; the events page reads only its page of
// the stream. Neither adds a store or a read path: the facet list filters the fold the
// inspector already caches, and the events page asks the ledger for a seq range, its
// ordinary replay.

// listPage is how many rows a list page shows.
const listPage = 300

// facetFilter is what the facet list is narrowed by; empty fields admit everything.
type facetFilter struct {
	Type, Status, Source string
}

func facetFilterOf(r *http.Request) facetFilter {
	q := r.URL.Query()
	return facetFilter{Type: q.Get("type"), Status: q.Get("status"), Source: q.Get("source")}
}

func (f facetFilter) admits(row facetRow) bool {
	return (f.Type == "" || string(row.Type) == f.Type) &&
		(f.Status == "" || string(row.Status) == f.Status) &&
		(f.Source == "" || row.Source == f.Source)
}

// query is the filter as URL parameters, for links that keep it.
func (f facetFilter) query(skip int) string {
	v := url.Values{}
	for k, val := range map[string]string{"type": f.Type, "status": f.Status, "source": f.Source} {
		if val != "" {
			v.Set(k, val)
		}
	}
	if skip > 0 {
		v.Set("skip", strconv.Itoa(skip))
	}
	if len(v) == 0 {
		return "/facets"
	}
	return "/facets?" + v.Encode()
}

// facetList is one page of the filtered facet list, newest first, and what is needed
// to say where it sits and to move.
type facetList struct {
	Rows              []facetRow
	Matched, From, To int
	Newer, Older      string
	Filter            facetFilter
	Types, Sources    []string
	Statuses          []string
}

func pageFacets(rows []facetRow, f facetFilter, skip int) facetList {
	types, sources, statuses := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var matched []facetRow
	for _, r := range rows {
		types[string(r.Type)], sources[r.Source], statuses[string(r.Status)] = true, true, true
		if f.admits(r) {
			matched = append(matched, r)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].FactTime != matched[j].FactTime {
			return matched[i].FactTime > matched[j].FactTime
		}
		return matched[i].ID > matched[j].ID
	})
	skip = max(0, min(skip, len(matched)))
	end := min(skip+listPage, len(matched))
	out := facetList{Rows: matched[skip:end], Matched: len(matched), From: skip + 1, To: end, Filter: f,
		Types: keys(types), Sources: keys(sources), Statuses: keys(statuses)}
	if skip > 0 {
		out.Newer = f.query(max(0, skip-listPage))
	}
	if end < len(matched) {
		out.Older = f.query(end)
	}
	return out
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// eventPage is which seqs an events page shows: the listPage events up to and
// including upto, newest first. before is the request's ?before=, the first seq it
// does not show; zero means the head.
func eventPage(head ledger.Watermark, before int64) (since, upto ledger.Watermark) {
	upto = head
	if before > 0 && ledger.Watermark(before-1) < head {
		upto = ledger.Watermark(before - 1)
	}
	since = 0
	if upto > listPage {
		since = upto - listPage
	}
	return since, upto
}

func newestFirst(evs []facet.AnamEvent) []facet.AnamEvent {
	out := make([]facet.AnamEvent, len(evs))
	for i, ev := range evs {
		out[len(evs)-1-i] = ev
	}
	return out
}

// skipOf reads ?skip=, ignoring what is not a non-negative number.
func skipOf(r *http.Request) int {
	n, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("skip")))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
