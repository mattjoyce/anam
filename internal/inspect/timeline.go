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
// every facet at its event-time, marked at its folded status, the dead ghosted —
// and the composed strip below it is the CANONICAL — the live-only value at W.
// Two surfaces, two names (§7), one screen.

// Band geometry, in viewBox units. Stems rise from the baseline to lane heights
// staggered per type slot, echoing the diagram and keeping same-day facets of
// different types out of each other's way.
const (
	bandW     = 1200.0
	bandH     = 240.0
	bandLeft  = 24.0
	bandRight = 24.0
	baseline  = 196.0
	diamondR  = 6.0
)

// laneY maps a type's slot to its stem height: four lanes, cycled.
var laneY = []float64{64, 100, 136, 168}

// slotOrder pins the four MVP types to categorical slots in fixed order
// (dataviz: hues are assigned, never cycled). Types beyond these are slotted
// deterministically after, in name order, by the caller.
var slotOrder = []facet.Type{
	"health.body-composition", "health.heart-rate", "health.sleep", "health.metric-group",
}

type tlPoint struct {
	X, Y  float64
	ID    string
	Slot  int
	Live  bool
	Title string
}

type tlTick struct {
	X     float64
	Label string
}

type tlLegendEntry struct {
	Type  facet.Type
	Slot  int
	Count int
}

type tlStripEntry struct {
	ID    string
	Slot  int
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

	Points []tlPoint
	Ticks  []tlTick
	Legend []tlLegendEntry

	Strip      []tlStripEntry
	StripCount int
	Zoomed     bool
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
		f, err := s.ledger.CurrentVersion(ctx, id)
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

	data := buildTimeline(res, all, from, to, zoomed)
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
	min, max := all[0].EventTime, all[0].EventTime
	for _, f := range all {
		if f.EventTime.Before(min) {
			min = f.EventTime
		}
		if f.EventTime.After(max) {
			max = f.EventTime
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

func buildTimeline(res fold.Result, all []facet.Facet, from, to time.Time, zoomed bool) timelineData {
	slots := slotFor(all)
	x := func(t time.Time) float64 {
		frac := float64(t.Sub(from)) / float64(to.Sub(from))
		return bandLeft + frac*(bandW-bandLeft-bandRight)
	}

	sort.Slice(all, func(i, j int) bool {
		if !all[i].EventTime.Equal(all[j].EventTime) {
			return all[i].EventTime.Before(all[j].EventTime)
		}
		return all[i].ID < all[j].ID
	})

	data := timelineData{
		W: res.W, Hash: res.Value.Hash(),
		Total: len(res.States), Live: len(res.Live),
		BandW: bandW, BandH: bandH, Baseline: baseline,
		PlotLeft: bandLeft, PlotRight: bandW - bandRight,
		WindowFromMS: from.UnixMilli(), WindowToMS: to.UnixMilli(),
		Zoomed: zoomed,
	}
	if zoomed {
		data.From = from.UTC().Format(time.RFC3339)
		data.To = to.UTC().Format(time.RFC3339)
	}

	counts := map[facet.Type]int{}
	// Types sharing a lane (slots cycle over four lanes) collide when their
	// event-times coincide — the generic metric-group carries several groups per
	// day. Stacked marks step their stems up, as the diagram varies its heights,
	// so every facet stays visible.
	stacked := map[[2]int]int{}
	for _, f := range all {
		counts[f.Type]++
		if f.EventTime.Before(from) || f.EventTime.After(to) {
			continue
		}
		st := res.States[f.ID]
		slot := slots[f.Type]
		px := x(f.EventTime)
		stackKey := [2]int{slot % len(laneY), int(px)}
		lift := float64(stacked[stackKey]) * (2*diamondR + 2)
		stacked[stackKey]++
		data.Points = append(data.Points, tlPoint{
			X: px, Y: laneY[slot%len(laneY)] - lift,
			ID: f.ID, Slot: slot, Live: st.Live(),
			Title: fmt.Sprintf("%s · %s · v%d %s · %s",
				f.EventTime.UTC().Format("2006-01-02"), f.Type, f.ArtifactVersion, st.Status, f.Provenance.SourceRef),
		})
		if st.Live() {
			data.Strip = append(data.Strip, tlStripEntry{
				ID: f.ID, Slot: slot,
				Title: fmt.Sprintf("%s · %s", f.EventTime.UTC().Format("2006-01-02"), f.Type),
			})
		}
	}
	data.StripCount = len(data.Strip)

	for typ, slot := range slots {
		data.Legend = append(data.Legend, tlLegendEntry{Type: typ, Slot: slot, Count: counts[typ]})
	}
	sort.Slice(data.Legend, func(i, j int) bool { return data.Legend[i].Slot < data.Legend[j].Slot })

	data.Ticks = monthTicks(from, to, x)
	return data
}

// slotFor assigns each type its categorical slot: the four known types in fixed
// order, anything else deterministically after, by name.
func slotFor(all []facet.Facet) map[facet.Type]int {
	slots := map[facet.Type]int{}
	for i, t := range slotOrder {
		slots[t] = i
	}
	var extra []facet.Type
	seen := map[facet.Type]bool{}
	for _, f := range all {
		if _, known := slots[f.Type]; !known && !seen[f.Type] {
			seen[f.Type] = true
			extra = append(extra, f.Type)
		}
	}
	sort.Slice(extra, func(i, j int) bool { return extra[i] < extra[j] })
	for i, t := range extra {
		slots[t] = len(slotOrder) + i
	}
	return slots
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
