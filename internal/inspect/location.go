package inspect

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// locationData is the /location page: a window of hours on one plane.
type locationData struct {
	Type       facet.Type
	Undrawable int
	W          ledger.Watermark
	From, To   string
	Device     string
	Devices    []string
	Track      *trackView
	Empty      string // why nothing is drawn, when nothing is
	Nav        tlNav
	// Zone is the zone a bare date in From or To is read in (card 236).
	Zone readZone
}

// location draws the live selected hours in a window. Without a window it
// shows the newest UTC day that has positions.
func (s *Server) location(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()

	res, err := s.read(ctx)
	if err != nil {
		httpError(w, err)
		return
	}
	var hours []facet.Facet
	for _, f := range res.Live {
		if f.Type == s.presentation.TrackType() {
			hours = append(hours, f)
		}
	}
	q := r.URL.Query()
	zone, ok := s.zoneOrFail(w, q)
	if !ok {
		return
	}
	device := q.Get("device")

	data := locationData{Type: s.presentation.TrackType(), W: res.W, Device: device, Zone: zone}
	if len(hours) == 0 {
		data.Empty = fmt.Sprintf("no %s facets are live", data.Type)
		s.render(w, "location.html", data)
		return
	}

	// The default window is the newest day with positions: a day is what the job
	// story asks to read, and the whole record would be one unreadable scribble.
	newest := hours[len(hours)-1].FactTime.UTC()
	from := time.Date(newest.Year(), newest.Month(), newest.Day(), 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	if fq, tq := q.Get("from"), q.Get("to"); fq != "" || tq != "" {
		if from, to, _, err = parseWindow(fq, tq, hours, zone.Loc); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	data.From, data.To = from.Format(time.RFC3339), to.Format(time.RFC3339)
	data.Nav = locationNav(from, to, device)

	pts, seen, omitted := windowPoints(hours, device, from, to)
	data.Undrawable = omitted
	if device != "" {
		// The device asked for stays selectable even with no fixes in this window,
		// so the select never shows "any" while the filter is on.
		seen[device] = true
	}
	for d := range seen {
		data.Devices = append(data.Devices, d)
	}
	sort.Strings(data.Devices)
	if len(pts) == 0 {
		data.Empty = "no positions in this window"
		s.render(w, "location.html", data)
		return
	}
	v := layoutTrack(pts, hourHue)
	v.Device = device
	v.Hours = countHours(pts)
	data.Track = &v
	s.render(w, "location.html", data)
}

// windowPoints gathers the fixes of the hours in [from, to) for one device, or
// every device when device is empty, and names every drawable device in the window.
// One plain condition per skip: gosec's analysis of a compound skip inside this
// loop ran for minutes (card 133).
func windowPoints(hours []facet.Facet, device string, from, to time.Time) ([]trackPoint, map[string]bool, int) {
	omitted := 0
	var pts []trackPoint
	seen := map[string]bool{}
	for _, f := range hours {
		if f.FactTime.Before(from) || !f.FactTime.Before(to) {
			continue
		}
		dev, p, err := parseTrack(f)
		if err != nil {
			omitted++
			continue // a payload the drawing cannot read is left out, not fatal
		}
		seen[dev] = true
		if device != "" && dev != device {
			continue
		}
		pts = append(pts, p...)
	}
	return pts, seen, omitted
}

func countHours(pts []trackPoint) int {
	seen := map[string]bool{}
	for _, p := range pts {
		seen[p.FacetID] = true
	}
	return len(seen)
}

// locationNav is the window moved by a day either way, and widened to a week.
func locationNav(from, to time.Time, device string) tlNav {
	width := to.Sub(from)
	link := func(f, t time.Time) string {
		q := url.Values{"from": {f.UTC().Format(time.RFC3339)}, "to": {t.UTC().Format(time.RFC3339)}}
		if device != "" {
			q.Set("device", device)
		}
		return "/location?" + q.Encode()
	}
	return tlNav{
		Left:  link(from.Add(-width), to.Add(-width)),
		Right: link(from.Add(width), to.Add(width)),
		In:    link(from.Add(width/4), to.Add(-width/4)),
		Out:   link(from.Add(-width/2), to.Add(width/2)),
	}
}
