package inspect

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The landing page draws the capture→compose picture over the live ledger
// (docs/assets/capture-compose-use.png): the capture band is the LEDGER surface —
// every facet at its fact-time, marked at its folded status, the dead ghosted —
// and the composed strip below it is the CANONICAL — the live-only value at W.
// Two surfaces, two names (§7), one screen.

// Band geometry, in viewBox units. Each family is a row (marks.go); the plot
// starts right of the row labels.
const (
	bandW     = 1200.0
	bandLeft  = 104.0
	bandRight = 24.0
	rowTop    = 30.0
	rowH      = 30.0
	axisH     = 28.0
)

type tlRow struct {
	Family, Label, Hue string
	Types              []facet.Type
	Y, Top, Height     float64
	Stripe             bool // every other row is shaded, for the eye to follow
}

type tlPoint struct {
	Path, Hue string
	ID        string
	Live      bool
	Title     string
}

type tlTick struct {
	X     float64
	Label string
}

type tlLegendEntry struct {
	Type    facet.Type
	Swatch  string // the type's mark, centred in a 16×16 box
	Class   string // the mark's CSS class: hue, and hollow for a reading
	Count   int
	Checked bool
}

type tlLegendGroup struct {
	Family, Label, Hue string
	Entries            []tlLegendEntry
}

type tlStripEntry struct {
	ID    string
	Hue   string
	Title string
}

type timelineData struct {
	W        ledger.Watermark
	Hash     string
	Total    int // logical facets in the ledger
	Live     int // live facets in the canonical
	From, To string

	// The rendered window as epoch milliseconds — the drag-select's x→time scale.
	WindowFromMS, WindowToMS int64

	BandW, BandH, Baseline, PlotLeft, PlotRight float64

	Rows   []tlRow
	Points []tlPoint
	Ticks  []tlTick
	Legend []tlLegendGroup

	Strip      []tlStripEntry
	StripCount int
	Zoomed     bool

	// Filtered is true when the type filter hides something; Status is the
	// status filter in force. Shown counts the marks drawn in the window.
	Filtered bool
	Status   statusFilter
	Shown    int

	// Nav is the window moved: the same read at another from/to, each a link, so
	// every gesture is a round trip and the band stays honest to the fold at W.
	// The type filter rides along on every link.
	Nav tlNav
}

// tlNav holds the moves a window can make, and the filter presets, as URLs.
type tlNav struct {
	Left, Right, In, Out          string
	AllTypes, EventsOnly          string
	AllStatus, LiveOnly, DeadOnly string
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	res, err := s.read(ctx)
	if err != nil {
		httpError(w, err)
		return
	}

	// The band needs the dead too — it is the ledger surface. Fetch them (few;
	// the live majority is already in hand).
	all := make([]facet.Facet, 0, len(res.States))
	all = append(all, res.Live...)
	for id, st := range res.States {
		if st.Live() {
			continue
		}
		f, err := s.ledger.Version(ctx, id, st.Version)
		if err != nil {
			httpError(w, err)
			return
		}
		all = append(all, f)
	}

	from, to, zoomed, err := parseWindow(r.URL.Query().Get("from"), r.URL.Query().Get("to"), all)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	status, err := parseStatus(r.URL.Query().Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	data := buildTimeline(res, all, from, to, zoomed, r.URL.Query()["type"], status)
	render(w, "timeline.html", data)
}

// parseWindow resolves the visible window: the caller's from/to when given (a bare
// date or RFC 3339; --to's bare date includes its whole day), otherwise the full
// extent of the data padded a little so edge diamonds don't sit on the frame.
func parseWindow(fromQ, toQ string, all []facet.Facet) (from, to time.Time, zoomed bool, err error) {
	if len(all) == 0 {
		// An empty ledger draws an empty band over an arbitrary week.
		from = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		return from, from.AddDate(0, 0, 7), false, nil
	}
	min, max := all[0].FactTime, all[0].FactTime
	for _, f := range all {
		if f.FactTime.Before(min) {
			min = f.FactTime
		}
		if f.FactTime.After(max) {
			max = f.FactTime
		}
	}
	pad := max.Sub(min) / 50
	if pad <= 0 {
		pad = 12 * time.Hour
	}
	from, to = min.Add(-pad), max.Add(pad)

	if fromQ != "" {
		t, _, err := parseInstant(fromQ)
		if err != nil {
			return time.Time{}, time.Time{}, false, fmt.Errorf("from: %w", err)
		}
		from, zoomed = t, true
	}
	if toQ != "" {
		t, dateOnly, err := parseInstant(toQ)
		if err != nil {
			return time.Time{}, time.Time{}, false, fmt.Errorf("to: %w", err)
		}
		if dateOnly {
			t = t.AddDate(0, 0, 1)
		}
		to, zoomed = t, true
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, false, fmt.Errorf("empty window: from %s is not before to %s", fromQ, toQ)
	}
	return from, to, zoomed, nil
}

// parseInstant reads a bound as a bare UTC date or an RFC 3339 instant.
func parseInstant(s string) (t time.Time, dateOnly bool, err error) {
	if t, err := time.ParseInLocation("2006-01-02", s, time.UTC); err == nil {
		return t, true, nil
	}
	t, err = time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("%q is neither 2006-01-02 nor RFC 3339", s)
	}
	return t, false, nil
}

func buildTimeline(res fold.Result, all []facet.Facet, from, to time.Time, zoomed bool, typeQ []string, status statusFilter) timelineData {
	x := func(t time.Time) float64 {
		frac := float64(t.Sub(from)) / float64(to.Sub(from))
		return bandLeft + frac*(bandW-bandLeft-bandRight)
	}

	sort.Slice(all, func(i, j int) bool {
		if !all[i].FactTime.Equal(all[j].FactTime) {
			return all[i].FactTime.Before(all[j].FactTime)
		}
		return all[i].ID < all[j].ID
	})

	// Rows and the legend come from every type in the ledger; the counts say how
	// many of each the status filter lets through, so the legend tells what the
	// band draws.
	counts := map[facet.Type]int{}
	seen := map[facet.Type]bool{}
	var types []facet.Type
	for _, f := range all {
		if !seen[f.Type] {
			seen[f.Type] = true
			types = append(types, f.Type)
		}
		if status.admits(res.States[f.ID].Live()) {
			counts[f.Type]++
		}
	}
	show, selected := typeFilter(typeQ, types)
	view := tlView{Types: selected, Status: status}

	// Rows come from every type in the ledger, not the filtered few, so a filter
	// hides marks without moving the rows that are left.
	rows, yOf := layoutRows(types)
	baseline := rowTop + float64(len(rows))*rowH

	data := timelineData{
		W: res.W, Hash: res.Value.Hash(),
		Total: len(res.States), Live: len(res.Live),
		BandW: bandW, BandH: baseline + axisH, Baseline: baseline,
		PlotLeft: bandLeft, PlotRight: bandW - bandRight,
		WindowFromMS: from.UnixMilli(), WindowToMS: to.UnixMilli(),
		Zoomed: zoomed, Rows: rows,
		Filtered: selected != nil, Status: status,
		Nav: navLinks(from, to, view),
	}
	if zoomed {
		data.From = from.UTC().Format(time.RFC3339)
		data.To = to.UTC().Format(time.RFC3339)
	}
	var events []facet.Type
	for _, t := range types {
		if isEvent(t) {
			events = append(events, t)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i] < events[j] })
	data.Nav.AllTypes = windowLink(from, to, zoomed, tlView{Status: status})
	data.Nav.EventsOnly = windowLink(from, to, zoomed, tlView{Types: events, Status: status})
	data.Nav.AllStatus = windowLink(from, to, zoomed, tlView{Types: selected})
	data.Nav.LiveOnly = windowLink(from, to, zoomed, tlView{Types: selected, Status: statusLive})
	data.Nav.DeadOnly = windowLink(from, to, zoomed, tlView{Types: selected, Status: statusDead})

	for _, f := range all {
		if !show[f.Type] || f.FactTime.Before(from) || f.FactTime.After(to) {
			continue
		}
		st := res.States[f.ID]
		if !status.admits(st.Live()) {
			continue
		}
		style := styleOf(f.Type)
		hue := hueClass(style.Family)
		mark := markClass(f.Type)
		when := f.FactTime.UTC().Format("2006-01-02 15:04Z")
		data.Points = append(data.Points, tlPoint{
			Path: markPath(style.Shape, x(f.FactTime), yOf[f.Type]),
			Hue:  mark, ID: f.ID, Live: st.Live(),
			Title: fmt.Sprintf("%s · %s · v%d %s · %s",
				when, f.Type, f.ArtifactVersion, st.Status, f.Provenance.SourceRef),
		})
		if st.Live() {
			data.Strip = append(data.Strip, tlStripEntry{
				ID: f.ID, Hue: hue, Title: when + " · " + string(f.Type),
			})
		}
	}
	data.Shown = len(data.Points)
	data.StripCount = len(data.Strip)

	for _, row := range rows {
		g := tlLegendGroup{Family: row.Family, Label: row.Label, Hue: row.Hue}
		for _, t := range row.Types {
			g.Entries = append(g.Entries, tlLegendEntry{
				Type: t, Swatch: markPath(styleOf(t).Shape, 8, 8.5), Class: markClass(t),
				Count: counts[t], Checked: show[t],
			})
		}
		data.Legend = append(data.Legend, g)
	}

	data.Ticks = monthTicks(from, to, x)
	return data
}

// layoutRows places each family's row and each type's sub-lane within it. Types
// in a row sit a few units apart, so a day's body-composition and weight do not
// land on the same spot.
func layoutRows(types []facet.Type) ([]tlRow, map[facet.Type]float64) {
	rows := rowsFor(types)
	yOf := map[facet.Type]float64{}
	for i := range rows {
		rows[i].Top, rows[i].Height = rowTop+float64(i)*rowH, rowH
		rows[i].Y = rows[i].Top + rowH/2
		rows[i].Stripe = i%2 == 0
		n := len(rows[i].Types)
		step := 0.0
		if n > 1 {
			step = min(7, 18/float64(n-1))
		}
		for j, t := range rows[i].Types {
			yOf[t] = rows[i].Y + (float64(j)-float64(n-1)/2)*step
		}
	}
	return rows, yOf
}

// monthTicks lays out at most ~8 month-boundary ticks across the window.
func monthTicks(from, to time.Time, x func(time.Time) float64) []tlTick {
	months := int(to.Sub(from).Hours()/(24*30)) + 1
	step := 1
	for _, s := range []int{1, 2, 3, 6, 12} {
		step = s
		if months/s <= 8 {
			break
		}
	}

	var ticks []tlTick
	t := time.Date(from.UTC().Year(), from.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	if t.Before(from) {
		t = t.AddDate(0, step, 0)
	}
	for ; !t.After(to); t = t.AddDate(0, step, 0) {
		label := t.Format("Jan 2006")
		if step >= 12 {
			label = t.Format("2006")
		}
		ticks = append(ticks, tlTick{X: x(t), Label: label})
	}
	return ticks
}
