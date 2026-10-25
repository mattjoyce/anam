package inspect

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The landing page draws the capture→compose picture over the live ledger
// (docs/assets/capture-compose-use.png): the capture band is the LEDGER surface —
// every facet at its fact-time, marked at its folded status — and the composed
// strip below it is the CANONICAL — the live-only value at W. Two surfaces, two
// names (§7), one screen. Since card 135 the band draws the live by default and
// the dead, ghosted, on request (?status=all or dead): the dead outnumber the live
// once a source is retired, and reading them costs a ledger fetch each.

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

	// Domain is the lane the row belongs to. Level 0 is the lane itself, which draws
	// the marks of all its types when closed (Types) and none when open; Level 1 is a
	// family row under an open lane (card 245).
	Domain string
	Level  int
	Open   bool
	// Empty says nothing is drawn in the lane in this window; Count is how many marks.
	Empty bool
	Count int
	// Toggle opens or closes the lane; "" for a lane a filter or the selection holds
	// open.
	Toggle string
}

type tlPoint struct {
	Path, Hue string
	ID        string
	Live      bool
	Title     string
	// X and Y are the mark's centre in viewBox units and MS its fact time as epoch
	// milliseconds: what the browser's selection needs (card 132) without
	// re-deriving the layout.
	X, Y float64
	MS   int64
	// Picked and Faded mark a saved selection drawn among the rest (card 206): the
	// facets it selects, and every other one.
	Picked, Faded bool
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
	Family, Label, Hue, Domain string
	Entries                    []tlLegendEntry
}

type tlStripEntry struct {
	ID    string
	Type  facet.Type
	Hue   string
	Title string
}

// tlTally is one type's share of a strip too large to draw square by square; its
// link is the window narrowed to that type.
type tlTally struct {
	Type  facet.Type
	Hue   string
	Count int
	Href  string
}

// stripMax is the most squares the strip draws; beyond it the page would grow with
// the record, so the strip tallies by type instead. Presentation (card 151).
const stripMax = 1500

type timelineData struct {
	// CanLink offers "link these…" on the page's own selection when the Workbench
	// enters links (card 199).
	CanLink bool

	W        ledger.Watermark
	Hash     string
	Total    int // logical facets in the ledger
	Live     int // live facets in the canonical
	From, To string

	// The rendered window as epoch milliseconds — the drag-select's x→time scale.
	WindowFromMS, WindowToMS int64

	BandW, BandH, Baseline, PlotLeft, PlotRight float64

	Rows []tlRow
	// OpenQ is the lanes the address opens, for the form to carry; Held is true while
	// any lane is open only because a filter or the selection touches it.
	OpenQ  []string
	Points []tlPoint
	Runs   []tlRun // dense stretches drawn as two ends and a line (card 135)
	Ticks  []tlTick
	Legend []tlLegendGroup

	Strip      []tlStripEntry
	StripCount int
	// Past stripMax live facets the strip tallies by type instead of drawing a
	// square each (card 135): Tally holds the counts, Strip is empty.
	Tally  []tlTally
	Zoomed bool

	// Filtered is true when the type filter hides something, None when it hides
	// everything; Status is the status filter in force. Shown counts the facets
	// drawn in the window, as marks or inside runs. Dead counts the ledger's dead
	// facets; DeadHidden says the status filter left them unread.
	Filtered   bool
	None       bool
	Status     statusFilter
	Shown      int
	Dead       int
	DeadHidden bool

	// Query is this view as a query string, what "make this my default" stores;
	// HasDefault says a default is set, IsDefault that this view is it.
	Query      string
	HasDefault bool
	IsDefault  bool

	// Nav is the window moved: the same read at another from/to, each a link, so
	// every gesture is a round trip and the band stays honest to the fold at W.
	// The type filter rides along on every link.
	Nav tlNav

	// Checkpoints is the bucket strip under the band and its header line (card
	// 21): every leaf bucket of the checkpoint layout at its state against the
	// log. Have is false when the inspector was given no log.
	Checkpoints tlCheckpoints

	// Sel is the saved selection drawn (card 206), nil for none; SelNav its links.
	Sel    *tlSelection
	SelNav tlSelNav
	ref    selRef

	// Zone is the zone a bare date in from or to is read in (card 236).
	Zone readZone
	// Selected is the types the type filter names (nil for all); SelectRange is the
	// select page over a zoomed window, "" when the window is the whole record's
	// (card 237).
	Selected    []facet.Type
	SelectRange string
}

// tlSelNav is the selection's links: the window drawn the other way, and without it.
type tlSelNav struct {
	Only, Highlight, Clear, Select string
}

// tlCheckpoints summarises the checkpoint log at W and carries the buckets that
// fall in the window, each as a segment of the strip.
type tlCheckpoints struct {
	Have    bool
	Buckets []tlBucket

	// Wrong is the log's refusal when the operational index is wrong (ErrIndexWrong):
	// the strip is drawn as one red band carrying these words, and nothing else of
	// the log is read, since none of it can be trusted until the operator rebuilds it.
	Wrong string

	Total, Sealed, Behind, Never int

	// Run is the last mint run at or below W, when there is one. LogSealed is
	// false when a mint crashed after writing, or no run was ever sealed: then
	// reads fold in full, and the strip says so.
	Run       int64
	HasRun    bool
	LogSealed bool
}

// tlBucket is one bucket as the strip draws it, clipped to the window.
type tlBucket struct {
	ID       string
	X, Width float64
	State    string
	Title    string
	Href     string
}

// tlNav holds the moves a window can make, and the filter presets, as URLs.
type tlNav struct {
	Left, Right, In, Out          string
	AllTypes, EventsOnly, NoTypes string
	AllStatus, LiveOnly, DeadOnly string
	OpenAll, CloseAll             string
}

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) {
	// A bare "/" opens the default view when one is kept (card 135); the redirect
	// keeps the URL honest about what is drawn.
	stored := defaultView(r)
	if r.URL.RawQuery == "" && len(stored) > 0 {
		http.Redirect(w, r, address("/", stored), http.StatusSeeOther)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	q := r.URL.Query()
	zone, ok := s.zoneOrFail(w, q)
	if !ok {
		return
	}
	status, err := parseStatus(q.Get("status"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := s.read(ctx)
	if err != nil {
		httpError(w, err)
		return
	}
	all, err := s.bandFacets(ctx, res, status)
	if err != nil {
		httpError(w, err)
		return
	}

	from, to, zoomed, err := parseWindow(q.Get("from"), q.Get("to"), all, zone.Loc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var sel *tlSelection
	if code := q.Get("sel"); code != "" {
		var ok bool
		if sel, all, from, to, ok = s.drawSelection(ctx, w, q, res, all, from, to, zoomed); !ok {
			return
		}
	}

	// The Definitions in force say what each type is (kind, family; card 176), so a
	// registered type draws on the right row before anyone edits the table.
	reg, err := s.definitions()
	if err != nil {
		httpError(w, err)
		return
	}
	data := buildTimeline(newStyles(reg), res, all, from, to, zoomed, q["type"], q["open"], status, sel)
	data.CanLink = s.enterable(catchup.EdgeType)
	data.Zone = zone
	if zoomed {
		data.SelectRange = selectRangeLink(from, to, data.Selected, zone)
	}
	if s.checkpoints != nil {
		states, summary, err := checkpointStates(ctx, s.checkpoints, res, s.buckets)
		switch {
		case errors.Is(err, ErrIndexWrong):
			data.Checkpoints = tlCheckpoints{Have: true, Wrong: err.Error()}
		case err != nil:
			httpError(w, err)
			return
		default:
			data.Checkpoints = bucketStrip(states, summary, res, from, to, data)
		}
	}
	data.Query = address("", cleanView(r.URL.RawQuery))
	data.HasDefault = len(stored) > 0
	data.IsDefault = data.HasDefault && address("", stored) == data.Query
	s.render(w, "timeline.html", data)
}

// bandFacets is what the band may draw under the status filter: the live (already
// in hand from the fold) and, only when the filter admits them, the dead, fetched
// from the ledger in one batch.
func (s *Server) bandFacets(ctx context.Context, res fold.Result, status statusFilter) ([]facet.Facet, error) {
	all := make([]facet.Facet, 0, len(res.States))
	if status != statusDead {
		all = append(all, res.Live...)
	}
	if !status.wantsDead() {
		return all, nil
	}
	var refs []ledger.VersionRef
	for id, st := range res.States {
		if st.Live() {
			continue
		}
		refs = append(refs, ledger.VersionRef{FacetID: id, Version: st.Version})
	}
	if len(refs) == 0 {
		return all, nil
	}
	dead, err := s.ledger.Versions(ctx, refs)
	if err != nil {
		return nil, fmt.Errorf("dead facets: %w", err)
	}
	return append(all, dead...), nil
}

// parseWindow resolves the visible window: the caller's from/to when given (a bare
// date, a day in loc, or RFC 3339; --to's bare date includes its whole day), otherwise the full
// extent of the data padded a little so edge diamonds don't sit on the frame.
func parseWindow(fromQ, toQ string, all []facet.Facet, loc *time.Location) (from, to time.Time, zoomed bool, err error) {
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
		t, _, err := clock.ParseBound(fromQ, loc)
		if err != nil {
			return time.Time{}, time.Time{}, false, fmt.Errorf("from: %w", err)
		}
		from, zoomed = t, true
	}
	if toQ != "" {
		t, dateOnly, err := clock.ParseBound(toQ, loc)
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

func buildTimeline(st styles, res fold.Result, all []facet.Facet, from, to time.Time, zoomed bool, typeQ, openQ []string, status statusFilter, sel *tlSelection) timelineData {
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
	show, selected, none := typeFilter(typeQ, types)
	ref := sel.ref()

	// Lanes come from every type in the ledger and the Definitions, not the filtered
	// few, so a filter hides marks without moving the rows that are left. A lane is
	// open when the address says so, or when the type filter or the selection touches
	// it, so what was asked for is not hidden behind a closed lane (card 245).
	domainLanes := st.lanesFor(types)
	laneIDs := map[string]bool{}
	for _, l := range domainLanes {
		laneIDs[l.ID] = true
	}
	explicit := openLanes(openQ, laneIDs)
	held := map[string]bool{}
	for _, t := range selected {
		held[domainOf(t)] = true
	}
	if sel != nil {
		for _, f := range all {
			if sel.Picked[f.ID] {
				held[domainOf(f.Type)] = true
			}
		}
	}
	isOpen := func(d string) bool { return held[d] || slices.Contains(explicit, d) }
	view := tlView{Types: selected, None: none, Status: status, Sel: ref, Open: explicit}
	rows, yOf := layoutLanes(domainLanes, isOpen)
	rowY := map[facet.Type]float64{}
	for _, row := range rows {
		for _, t := range row.Types {
			rowY[t] = row.Y
		}
	}
	baseline := rowTop + float64(len(rows))*rowH

	data := timelineData{
		W: res.W, Hash: res.Value.Hash(),
		Total: len(res.States), Live: len(res.Live),
		BandW: bandW, BandH: baseline + axisH, Baseline: baseline,
		PlotLeft: bandLeft, PlotRight: bandW - bandRight,
		WindowFromMS: from.UnixMilli(), WindowToMS: to.UnixMilli(),
		Zoomed: zoomed, Rows: rows,
		Filtered: selected != nil || none, None: none, Status: status,
		Dead: len(res.States) - len(res.Live), DeadHidden: !status.wantsDead() && len(res.States) > len(res.Live),
		Nav: navLinks(from, to, view),
		Sel: sel, ref: ref, Selected: selected, OpenQ: explicit,
	}
	if zoomed {
		data.From = from.UTC().Format(time.RFC3339)
		data.To = to.UTC().Format(time.RFC3339)
	}
	var events []facet.Type
	for _, t := range types {
		if st.isEvent(t) {
			events = append(events, t)
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i] < events[j] })
	data.Nav.AllTypes = windowLink(from, to, zoomed, tlView{Status: status, Sel: ref, Open: explicit})
	data.Nav.EventsOnly = windowLink(from, to, zoomed, tlView{Types: events, Status: status, Sel: ref, Open: explicit})
	data.Nav.NoTypes = windowLink(from, to, zoomed, tlView{None: true, Status: status, Sel: ref, Open: explicit})
	data.Nav.AllStatus = windowLink(from, to, zoomed, tlView{Types: selected, None: none, Status: statusAll, Sel: ref, Open: explicit})
	data.Nav.LiveOnly = windowLink(from, to, zoomed, tlView{Types: selected, None: none, Sel: ref, Open: explicit})
	data.Nav.DeadOnly = windowLink(from, to, zoomed, tlView{Types: selected, None: none, Status: statusDead, Sel: ref, Open: explicit})
	opened, closed := view, view
	opened.Open, closed.Open = sortedKeys(laneIDs), nil
	data.Nav.OpenAll = windowLink(from, to, zoomed, opened)
	data.Nav.CloseAll = windowLink(from, to, zoomed, closed)
	if sel != nil {
		data.SelNav = selNav(from, to, zoomed, view)
	}

	// Each lane (a type, live or dead) collects its marks in time order; runs are
	// found per lane, so a dead stretch never hides inside a live one.
	// A closed domain is one lane for all its types, so its marks join into runs
	// across types; an open domain keeps a lane for each type.
	type laneKey struct {
		lane         string
		live, picked bool
	}
	shownIn := map[string]int{}
	lanes := map[laneKey][]laneMark{}
	var laneOrder []laneKey
	for _, f := range all {
		if !show[f.Type] || f.FactTime.Before(from) || f.FactTime.After(to) {
			continue
		}
		state := res.States[f.ID]
		if !status.admits(state.Live()) {
			continue
		}
		style := st.of(f.Type)
		hue := hueOf(f.Type)
		mark := st.markClass(f.Type)
		when := f.FactTime.UTC().Format("2006-01-02 15:04Z")
		px, py := x(f.FactTime), yOf[f.Type]
		picked := sel != nil && sel.Picked[f.ID]
		laneID := string(f.Type)
		if !isOpen(domainOf(f.Type)) {
			laneID = "~" + domainOf(f.Type)
		}
		key := laneKey{laneID, state.Live(), picked}
		if _, ok := lanes[key]; !ok {
			laneOrder = append(laneOrder, key)
		}
		lanes[key] = append(lanes[key], laneMark{at: f.FactTime, typ: f.Type, below: py > rowY[f.Type], point: tlPoint{
			Path: markPath(style.Shape, px, py),
			Hue:  mark, ID: f.ID, Live: state.Live(),
			Title: fmt.Sprintf("%s · %s · v%d %s · %s",
				when, f.Type, f.ArtifactVersion, state.Status, f.Provenance.SourceRef),
			X: px, Y: py, MS: f.FactTime.UnixMilli(),
			Picked: picked && sel.Highlight(), Faded: !picked && sel.Highlight(),
		}})
		data.Shown++
		shownIn[domainOf(f.Type)]++
		if picked {
			sel.Count++
		}
		if state.Live() {
			data.Strip = append(data.Strip, tlStripEntry{
				ID: f.ID, Type: f.Type, Hue: hue, Title: when + " · " + string(f.Type),
			})
		}
	}
	for _, key := range laneOrder {
		points, runs := laneRuns(lanes[key], view)
		data.Points = append(data.Points, points...)
		data.Runs = append(data.Runs, runs...)
	}
	data.StripCount = len(data.Strip)
	if data.StripCount > stripMax {
		data.Tally = tallyStrip(st, data.Strip, types, from, to, zoomed, view)
		data.Strip = nil
	}

	// The legend lists every family whether its lane is open or not, so a type can be
	// ticked without opening anything.
	for _, l := range domainLanes {
		for _, row := range l.Families {
			g := tlLegendGroup{Family: row.Family, Label: row.Label, Hue: row.Hue, Domain: l.ID}
			for _, t := range row.Types {
				g.Entries = append(g.Entries, tlLegendEntry{
					Type: t, Swatch: markPath(st.of(t).Shape, 8, 8.5), Class: st.markClass(t),
					Count: counts[t], Checked: show[t],
				})
			}
			data.Legend = append(data.Legend, g)
		}
	}

	for i := range data.Rows {
		row := &data.Rows[i]
		if row.Level != 0 {
			continue
		}
		row.Count = shownIn[row.Domain]
		row.Empty = row.Count == 0
		if held[row.Domain] {
			continue
		}
		toggled := view
		toggled.Open = toggleLane(explicit, row.Domain)
		row.Toggle = windowLink(from, to, zoomed, toggled)
	}

	data.Ticks = monthTicks(from, to, x)
	return data
}

// bucketStrip places the buckets that overlap the window on the band's x scale,
// each a link that zooms the band to the bucket, and carries the summary through.
func bucketStrip(states []bucketState, summary tlCheckpoints, res fold.Result, from, to time.Time, data timelineData) tlCheckpoints {
	x := func(t time.Time) float64 {
		frac := float64(t.Sub(from)) / float64(to.Sub(from))
		return data.PlotLeft + frac*(data.PlotRight-data.PlotLeft)
	}
	view := tlView{Status: data.Status, Sel: data.ref, Open: data.OpenQ}
	for _, st := range states {
		lo, hi := st.Bucket.From(), st.Bucket.To()
		if lo.Before(from) {
			lo = from
		}
		if hi.After(to) {
			hi = to
		}
		if !lo.Before(hi) {
			continue
		}
		width := x(hi) - x(lo)
		if width < 1 {
			width = 1
		}
		summary.Buckets = append(summary.Buckets, tlBucket{
			ID: st.Bucket.ID(), X: x(lo), Width: width, State: st.State,
			Title: bucketTitle(st, int64(res.W)),
			Href:  windowLink(st.Bucket.From(), st.Bucket.To(), true, view),
		})
	}
	return summary
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

// tallyStrip counts a strip by type, in the legend's order, each count linking to
// the window narrowed to its type.
func tallyStrip(st styles, strip []tlStripEntry, types []facet.Type, from, to time.Time, zoomed bool, view tlView) []tlTally {
	counts := map[facet.Type]int{}
	for _, e := range strip {
		counts[e.Type]++
	}
	var tally []tlTally
	for _, t := range types {
		n := counts[t]
		if n == 0 {
			continue
		}
		tally = append(tally, tlTally{
			Type: t, Hue: hueOf(t), Count: n,
			Href: windowLink(from, to, zoomed, tlView{Types: []facet.Type{t}, Status: view.Status, Sel: view.Sel, Open: view.Open}),
		})
	}
	return tally
}
