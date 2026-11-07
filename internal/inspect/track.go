package inspect

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/presentation"
)

// A selected track facet (card 130) is one UTC hour of one phone's fixes. The
// inspector draws those fixes as a track without a map (card 133): a local flat
// projection, metres east and north of the track's centre, the fixes joined in time
// order, each ringed by its accuracy at scale, with a scale bar. Nothing is fetched
// from outside the page: a tile server would learn the viewed area from every tile
// URL, which the rest of the location work takes care never to leak. The shape of
// the hour, honestly, not a street map.

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

// parseTrack validates one complete hourly facet. Errors never contain payload data.
func parseTrack(f facet.Facet) (device string, pts []trackPoint, err error) {
	invalid := func() (string, []trackPoint, error) {
		return "", nil, fmt.Errorf("%s: undrawable hourly track", f.Type)
	}
	if f.FactTime.IsZero() || !f.FactTime.Equal(f.FactTime.UTC().Truncate(time.Hour)) {
		return invalid()
	}
	var payload map[string]json.RawMessage
	var points []map[string]json.RawMessage
	if json.Unmarshal(f.Payload.Bytes(), &payload) != nil {
		return invalid()
	}
	device, ok := trackString(payload["device"])
	if !ok || device == "" || json.Unmarshal(payload["points"], &points) != nil || len(points) == 0 {
		return invalid()
	}
	end := f.FactTime.Add(time.Hour)

	for _, p := range points {
		lat, latOK := trackNumber(p["lat"], false)
		lon, lonOK := trackNumber(p["lon"], false)
		acc, accOK := trackNumber(p["acc"], true)
		vel, velOK := trackNumber(p["vel"], true)
		if !latOK || !lonOK || lat < -90 || lat > 90 || lon < -180 || lon > 180 || !accOK || acc < 0 || !velOK || vel < 0 {
			return invalid()
		}
		text, ok := trackString(p["t"])
		t, err := time.Parse(time.RFC3339, text)
		if !ok || err != nil || t.Before(f.FactTime) || !t.Before(end) {
			return invalid()
		}
		pts = append(pts, trackPoint{T: t, Lat: lat, Lon: lon, Acc: acc, Vel: vel, FacetID: f.ID, Device: device})
	}
	// Even finite inputs can overflow projected extents or scale arithmetic.
	if v := layoutTrack(pts, hourHue); v.Count == 0 {
		return invalid()
	}
	return device, pts, nil
}

func trackString(raw json.RawMessage) (string, bool) {
	var value *string
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return "", false
	}
	return *value, true
}

func trackNumber(raw json.RawMessage, optional bool) (float64, bool) {
	if len(raw) == 0 {
		return 0, optional
	}
	var value *float64
	if json.Unmarshal(raw, &value) != nil {
		return 0, false
	}
	if value == nil {
		return 0, optional
	}
	return *value, finite(*value)
}

func finite(values ...float64) bool {
	for _, v := range values {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return false
		}
	}
	return true
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
	for _, p := range pts {
		if !finite(p.Lat, p.Lon, p.Acc, p.Vel) || p.Acc < 0 || p.Vel < 0 {
			return trackView{}
		}
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
	if !finite(2*halfE, 2*halfN) {
		return trackView{}
	}
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

// facetTrack draws one selected facet for its page; nil for any other type,
// or a payload the drawing cannot read.
func facetTrack(f facet.Facet, policy presentation.Value) *trackView {
	if policy.TrackType() == "" || f.Type != policy.TrackType() {
		return nil
	}
	device, pts, err := parseTrack(f)
	if err != nil || len(pts) == 0 {
		return nil
	}
	v := layoutTrack(pts, func(trackPoint) string { return policy.Hue(domainOf(f.Type)) })
	v.Device, v.Hours = device, 1
	return &v
}
