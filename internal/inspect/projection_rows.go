package inspect

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// The projection page's two views. The summary is one line per type and costs no
// ledger read; the rows view draws one type's rows of one standing, so the page weighs
// what the person chose to open and not the whole record (card 201).

// rowLimit is the most rows the rows view draws. It is how big a page is, which is
// presentation (AGENTS.md, card 151): a page of thousands of rows is the problem this
// view exists to avoid, and a narrower window is the answer it asks for.
const rowLimit = 500

// notLive names the summary line for facets that are no longer live. The fold holds a
// dead facet's status and version but not its type, and finding the type would be a
// ledger read per dead facet, so the summary does not attribute them to one.
const notLive = "(not live)"

// typeSummary is one line of the summary: what the selected projection serves and
// leaves out of one type, and how many versions of that type are no longer in force.
type typeSummary struct {
	Type                         string
	Served, Excluded, Superseded int
}

// Link is the address of the rows behind one count, keeping the projection and the
// as-of the page is read at.
func (d projectionData) Link(typ, show string) string {
	q := d.query()
	q.Set("type", typ)
	q.Set("show", show)
	return "/projection?" + q.Encode()
}

// SummaryLink is the address of the summary at the same projection and as-of.
func (d projectionData) SummaryLink() string { return "/projection?" + d.query().Encode() }

// NowLink is the address of the same view at the head, without the as-of.
func (d projectionData) NowLink() string {
	d.AsOfSet = false
	q := d.query()
	if d.Type != "" {
		q.Set("type", d.Type)
		q.Set("show", d.Show)
	}
	return "/projection?" + q.Encode()
}

// Truncated reports whether the rows view matched more rows than it drew.
func (d projectionData) Truncated() bool { return d.RowsTotal > len(d.Rows) }

func (d projectionData) query() url.Values {
	q := url.Values{"p": {d.Selected}}
	if d.AsOfSet {
		q.Set("asof", d.AsOf)
	}
	return q
}

// view is the rows view's request, read from the query string.
type view struct {
	typ, show string
	from, to  string // fact-time bounds as clock.Format strings; to is exclusive
	rawFrom   string
	rawTo     string
}

var errBadView = errors.New("bad view")

func viewOf(q url.Values) (view, error) {
	v := view{typ: q.Get("type"), show: q.Get("show"), rawFrom: q.Get("from"), rawTo: q.Get("to")}
	if v.typ == "" {
		return v, nil
	}
	switch v.show {
	case "":
		v.show = "served"
	case "served", "excluded", "superseded":
	default:
		return v, fmt.Errorf("%w: show %q is not served, excluded or superseded", errBadView, v.show)
	}
	if v.rawFrom != "" {
		t, err := time.Parse(time.DateOnly, v.rawFrom)
		if err != nil {
			return v, fmt.Errorf("%w: from %q is not YYYY-MM-DD", errBadView, v.rawFrom)
		}
		v.from = clock.Format(t)
	}
	if v.rawTo != "" {
		t, err := time.Parse(time.DateOnly, v.rawTo)
		if err != nil {
			return v, fmt.Errorf("%w: to %q is not YYYY-MM-DD", errBadView, v.rawTo)
		}
		// The whole of the last day, so "to 2026-07-05" includes the 5th.
		v.to = clock.Format(t.AddDate(0, 0, 1))
	}
	return v, nil
}

// in reports whether a fact time falls inside the window.
func (v view) in(factTime string) bool {
	return (v.from == "" || factTime >= v.from) && (v.to == "" || factTime < v.to)
}

// fill answers the request. The summary is counts and nothing is read from the
// ledger for it; the rows view draws one type's rows of one standing, and reads
// versions no longer in force only when that is the standing asked for.
func (d *projectionData) fill(ctx context.Context, l Ledger, res fold.Result, v view) error {
	live, err := d.judgeLive(res)
	if err != nil {
		return err
	}
	superseded := supersededByType(res)
	for _, n := range superseded {
		d.NSuperseded += n
	}
	if v.typ == "" {
		d.Summary = summarise(live, superseded)
		return nil
	}
	return d.fillRows(ctx, l, res, live, v)
}

// supersededByType counts, from the fold's state alone, the versions no longer in
// force: a facet at version n has n-1 earlier versions, and a facet that is no longer
// live has its current version out of force as well. The count equals what
// fold.Displaced would list, without reading one version from the ledger.
//
// A renamed facet's earlier versions count under its current type, because the fold
// does not hold what type an earlier version had and finding out is the read this
// avoids. A facet that is no longer live has no type in the fold, so its versions
// count under notLive.
func supersededByType(res fold.Result) map[string]int {
	typeOf := make(map[string]facet.Type, res.Value.Len())
	for _, e := range res.Value.Entries() {
		typeOf[e.FacetID] = e.Type
	}
	out := map[string]int{}
	for id, st := range res.States {
		n := st.Version - 1
		typ := string(typeOf[id])
		if !st.Live() {
			typ, n = notLive, n+1
		}
		if n > 0 {
			out[typ] += n
		}
	}
	return out
}

// summarise counts the live rows' standings per type and joins the superseded counts.
func summarise(live []projVerdictRow, superseded map[string]int) []typeSummary {
	by := map[string]*typeSummary{}
	line := func(typ string) *typeSummary {
		if by[typ] == nil {
			by[typ] = &typeSummary{Type: typ}
		}
		return by[typ]
	}
	for _, r := range live {
		if r.Standing == "served" {
			line(string(r.Type)).Served++
		} else {
			line(string(r.Type)).Excluded++
		}
	}
	for typ, n := range superseded {
		line(typ).Superseded = n
	}
	out := make([]typeSummary, 0, len(by))
	for _, x := range by {
		out = append(out, *x)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// fillRows draws one type's rows of one standing inside the window, earliest first,
// and says how many match so the template can ask for a narrower window.
func (d *projectionData) fillRows(ctx context.Context, l Ledger, res fold.Result, live []projVerdictRow, v view) error {
	var rows []projVerdictRow
	if v.show == "superseded" {
		// Only now are versions read, and only the ones of this type's facets. The
		// type is the facet's current one, as in the summary's counts.
		typeOf := make(map[string]facet.Type, res.Value.Len())
		for _, e := range res.Value.Entries() {
			typeOf[e.FacetID] = e.Type
		}
		keep := func(id string) bool {
			if st := res.States[id]; !st.Live() {
				return v.typ == notLive
			}
			return string(typeOf[id]) == v.typ
		}
		displaced, err := fold.DisplacedWhere(ctx, l, res, keep)
		if err != nil {
			return err
		}
		for _, x := range displaced {
			if r := displacedRow(x); v.in(r.FactTime) {
				rows = append(rows, r)
			}
		}
	} else {
		for _, r := range live {
			if string(r.Type) == v.typ && r.Standing == v.show && v.in(r.FactTime) {
				rows = append(rows, r)
			}
		}
	}

	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch {
		case a.FactTime != b.FactTime:
			return a.FactTime < b.FactTime
		case a.ID != b.ID:
			return a.ID < b.ID
		default:
			return a.Version < b.Version
		}
	})
	d.RowsTotal = len(rows)
	if len(rows) > rowLimit {
		rows = rows[:rowLimit]
	}
	d.Rows = rows
	return nil
}
