package inspect

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// A location.track facet (card 130) is one UTC hour of one phone's fixes. The
// inspector draws those fixes as a track without a map (card 133): a local flat
// projection, metres east and north of the track's centre, the fixes joined in time
// order, each ringed by its accuracy at scale, with a scale bar. Nothing is fetched
// from outside the page: a tile server would learn the viewed area from every tile
// URL, which the rest of the location work takes care never to leak. The shape of
// the hour, honestly, not a street map.

// typeLocationTrack is the facet type the drawing reads.
const typeLocationTrack facet.Type = "location.track"

// Track geometry, in viewBox units.
const (
	trackW   = 800.0
	trackH   = 480.0
	trackPad = 36.0
	// Metres per degree of latitude; longitude is scaled by cos(latitude).
	metresPerDegree = 111320.0
)

// trackPoint is one fix as the payload wrote it, with the fields the drawing uses.
type trackPoint struct {
	T   time.Time
	Lat float64
	Lon float64
	Acc float64 // metres; 0 when absent
	Vel float64 // km/h; 0 when absent
	// FacetID is the hour the fix belongs to, so a window's drawing links back;
	// Device is the phone, so a window of several draws one line each.
	FacetID string
	Device  string
}

// trackHour is a location.track payload, as much of it as the drawing reads.
type trackHour struct {
	ID     string `json:"id"`
	Hour   string `json:"hour"`
	Device string `json:"device"`
	Points []struct {
		T   string   `json:"t"`
		Lat *float64 `json:"lat"`
		Lon *float64 `json:"lon"`
		Acc *float64 `json:"acc"`
		Vel *float64 `json:"vel"`
	} `json:"points"`
}

// parseTrack reads a location.track facet's fixes. A payload the drawing cannot read
// is an error, and the page falls back to the JSON alone.
func parseTrack(f facet.Facet) (device string, pts []trackPoint, err error) {
	var h trackHour
	if err := json.Unmarshal(f.Payload.Bytes(), &h); err != nil {
		return "", nil, fmt.Errorf("location.track payload: %w", err)
	}
	for i, p := range h.Points {
		if p.Lat == nil || p.Lon == nil {
			return "", nil, fmt.Errorf("location.track point %d: no lat/lon", i)
		}
		t, err := time.Parse(time.RFC3339, p.T)
		if err != nil {
			return "", nil, fmt.Errorf("location.track point %d: %w", i, err)
		}
		pt := trackPoint{T: t, Lat: *p.Lat, Lon: *p.Lon, FacetID: f.ID, Device: h.Device}
		if p.Acc != nil && *p.Acc > 0 {
			pt.Acc = *p.Acc
		}
		if p.Vel != nil && *p.Vel > 0 {
			pt.Vel = *p.Vel
		}
		pts = append(pts, pt)
	}
	return h.Device, pts, nil
}

// trackFix is one fix laid out on the plane.
type trackFix struct {
	X, Y    float64
	R       float64 // the accuracy ring's radius in viewBox units; 0 when unknown
	Title   string
	FacetID string
	Hue     string
}

// trackView is a drawn track: what the template needs and nothing else.
type trackView struct {
	W, H          float64
	Paths         []string // each device's fixes joined in time order
	Fixes         []trackFix
	First, Last   trackFix
	ScaleX        float64 // the scale bar's left end
	ScaleY        float64
	ScaleLen      float64 // its length in viewBox units
	ScaleEnd      float64 // its right end
	NorthX        float64 // where the north arrow sits
	ScaleLabel    string
	Count         int
	Span          string // the drawn extent, in metres, for the caption
	FromT, ToT    string
	Hours         int
	Device        string
	Devices       []string
	MetresPerUnit float64
}

// layoutTrack projects fixes onto the plane. Hues colour by hour of the day so a
// window of hours reads as a sequence; a single hour is one hue.
func layoutTrack(pts []trackPoint, hueOf func(trackPoint) string) trackView {
	v := trackView{W: trackW, H: trackH, Count: len(pts)}
	if len(pts) == 0 {
		return v
	}
	sort.SliceStable(pts, func(i, j int) bool { return pts[i].T.Before(pts[j].T) })

	// A local flat projection about the centre: good to well under a percent over
	// the few kilometres an hour or a day covers.
	var minLat, maxLat, minLon, maxLon = pts[0].Lat, pts[0].Lat, pts[0].Lon, pts[0].Lon
	for _, p := range pts {
		minLat, maxLat = math.Min(minLat, p.Lat), math.Max(maxLat, p.Lat)
		minLon, maxLon = math.Min(minLon, p.Lon), math.Max(maxLon, p.Lon)
	}
	lat0, lon0 := (minLat+maxLat)/2, (minLon+maxLon)/2
	cos := math.Cos(lat0 * math.Pi / 180)
	east := func(lon float64) float64 { return (lon - lon0) * cos * metresPerDegree }
	north := func(lat float64) float64 { return (lat - lat0) * metresPerDegree }

	// Fit the extent, accuracy rings included, keeping metres equal on both axes.
	var halfE, halfN float64
	for _, p := range pts {
		halfE = math.Max(halfE, math.Abs(east(p.Lon))+p.Acc)
		halfN = math.Max(halfN, math.Abs(north(p.Lat))+p.Acc)
	}
	halfE, halfN = math.Max(halfE, 15), math.Max(halfN, 15) // a still hour is still a place
	unitsPerMetre := math.Min((trackW/2-trackPad)/halfE, (trackH/2-trackPad)/halfN)
	v.MetresPerUnit = 1 / unitsPerMetre
	x := func(p trackPoint) float64 { return trackW/2 + east(p.Lon)*unitsPerMetre }
	y := func(p trackPoint) float64 { return trackH/2 - north(p.Lat)*unitsPerMetre }

	paths := map[string]*strings.Builder{}
	var devices []string
	for _, p := range pts {
		fx := trackFix{X: x(p), Y: y(p), R: p.Acc * unitsPerMetre, FacetID: p.FacetID, Hue: hueOf(p)}
		title := p.T.UTC().Format("2006-01-02 15:04:05Z")
		if p.Acc > 0 {
			title += fmt.Sprintf(" · ±%.0f m", p.Acc)
		}
		if p.Vel > 0 {
			title += fmt.Sprintf(" · %.0f km/h", p.Vel)
		}
		fx.Title = title
		v.Fixes = append(v.Fixes, fx)
		b, ok := paths[p.Device]
		if !ok {
			b = &strings.Builder{}
			paths[p.Device] = b
			devices = append(devices, p.Device)
			fmt.Fprintf(b, "M%.1f %.1f", fx.X, fx.Y)
		} else {
			fmt.Fprintf(b, "L%.1f %.1f", fx.X, fx.Y)
		}
	}
	for _, d := range devices {
		v.Paths = append(v.Paths, paths[d].String())
	}
	v.First, v.Last = v.Fixes[0], v.Fixes[len(v.Fixes)-1]
	v.FromT = pts[0].T.UTC().Format("2006-01-02 15:04Z")
	v.ToT = pts[len(pts)-1].T.UTC().Format("15:04Z")
	if pts[0].T.UTC().YearDay() != pts[len(pts)-1].T.UTC().YearDay() {
		v.ToT = pts[len(pts)-1].T.UTC().Format("2006-01-02 15:04Z")
	}

	// The scale bar: a round number of metres about a quarter of the width.
	target := (trackW / 4) * v.MetresPerUnit
	metres := niceMetres(target)
	v.ScaleLen = metres * unitsPerMetre
	v.ScaleX, v.ScaleY = trackPad, trackH-trackPad/2
	v.ScaleEnd, v.NorthX = v.ScaleX+v.ScaleLen, trackW-trackPad
	v.ScaleLabel = fmtMetres(metres)
	v.Span = fmtMetres(2*halfE) + " × " + fmtMetres(2*halfN)
	return v
}

// niceMetres rounds a length down to 1, 2 or 5 times a power of ten.
func niceMetres(m float64) float64 {
	if m <= 0 {
		return 10
	}
	p := math.Pow(10, math.Floor(math.Log10(m)))
	switch {
	case m >= 5*p:
		return 5 * p
	case m >= 2*p:
		return 2 * p
	default:
		return p
	}
}

func fmtMetres(m float64) string {
	if m >= 1000 {
		return fmt.Sprintf("%.3g km", m/1000)
	}
	return fmt.Sprintf("%.0f m", m)
}

// hourHue colours a fix by its hour of the UTC day, cycling the eight palette hues,
// so a day's track reads as a sequence without a legend.
func hourHue(p trackPoint) string {
	return fmt.Sprintf("h%d", p.T.UTC().Hour()%8)
}

// facetTrack draws one location.track facet for its page; nil for any other type,
// or a payload the drawing cannot read.
func facetTrack(f facet.Facet) *trackView {
	if f.Type != typeLocationTrack {
		return nil
	}
	device, pts, err := parseTrack(f)
	if err != nil || len(pts) == 0 {
		return nil
	}
	v := layoutTrack(pts, func(trackPoint) string { return "h1" })
	v.Device, v.Hours = device, 1
	return &v
}

// locationData is the /location page: a window of hours on one plane.
type locationData struct {
	W        ledger.Watermark
	From, To string
	Device   string
	Devices  []string
	Track    *trackView
	Empty    string // why nothing is drawn, when nothing is
	Nav      tlNav
}

// location draws the live location.track hours in a window. Without a window it
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
	devices := map[string]bool{}
	for _, f := range res.Live {
		if f.Type == typeLocationTrack {
			hours = append(hours, f)
		}
	}
	q := r.URL.Query()
	device := q.Get("device")

	data := locationData{W: res.W, Device: device}
	if len(hours) == 0 {
		data.Empty = "no location.track facets are live"
		render(w, "location.html", data)
		return
	}

	// The default window is the newest day with positions: a day is what the job
	// story asks to read, and the whole record would be one unreadable scribble.
	newest := hours[len(hours)-1].FactTime.UTC()
	from := time.Date(newest.Year(), newest.Month(), newest.Day(), 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, 1)
	if fq, tq := q.Get("from"), q.Get("to"); fq != "" || tq != "" {
		if from, to, _, err = parseWindow(fq, tq, hours); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	data.From, data.To = from.Format(time.RFC3339), to.Format(time.RFC3339)
	data.Nav = locationNav(from, to, device)

	pts, seen := windowPoints(hours, device, from, to)
	for d := range seen {
		devices[d] = true
	}
	for d := range devices {
		data.Devices = append(data.Devices, d)
	}
	sort.Strings(data.Devices)
	if len(pts) == 0 {
		data.Empty = "no positions in this window"
		render(w, "location.html", data)
		return
	}
	v := layoutTrack(pts, hourHue)
	v.Device = device
	v.Hours = countHours(pts)
	data.Track = &v
	render(w, "location.html", data)
}

// windowPoints gathers the fixes of the hours in [from, to) for one device, or
// every device when device is empty, and names every device seen in any hour.
// One plain condition per skip: gosec's analysis of a compound skip inside this
// loop ran for minutes (card 133).
func windowPoints(hours []facet.Facet, device string, from, to time.Time) ([]trackPoint, map[string]bool) {
	var pts []trackPoint
	seen := map[string]bool{}
	for _, f := range hours {
		dev, p, err := parseTrack(f)
		if err != nil {
			continue // a payload the drawing cannot read is left out, not fatal
		}
		seen[dev] = true
		if device != "" && dev != device {
			continue
		}
		if f.FactTime.Before(from) {
			continue
		}
		if !f.FactTime.Before(to) {
			continue
		}
		pts = append(pts, p...)
	}
	return pts, seen
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
		u := "/location?from=" + f.UTC().Format(time.RFC3339) + "&to=" + t.UTC().Format(time.RFC3339)
		if device != "" {
			u += "&device=" + device
		}
		return u
	}
	return tlNav{
		Left:  link(from.Add(-width), to.Add(-width)),
		Right: link(from.Add(width), to.Add(width)),
		In:    link(from.Add(width/4), to.Add(-width/4)),
		Out:   link(from.Add(-width/2), to.Add(width/2)),
	}
}
