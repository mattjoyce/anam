package inspect

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

const validTrack = `{"device":"phone","points":[{"t":"2026-10-12T02:05:00Z","lat":-33.8,"lon":151.2,"acc":14,"vel":12}]}`

func TestTrackValidation(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)
	tests := []struct {
		name, payload string
		at            time.Time
		valid         bool
	}{
		{"valid", validTrack, at, true},
		{"offset instant", strings.Replace(validTrack, "02:05:00Z", "13:05:00+11:00", 1), at, true},
		{"hour start", strings.Replace(validTrack, "02:05:00Z", "02:00:00Z", 1), at, true},
		{"last nanosecond", strings.Replace(validTrack, "02:05:00Z", "02:59:59.999999999Z", 1), at, true},
		{"next hour", strings.Replace(validTrack, "02:05:00Z", "03:00:00Z", 1), at, false},
		{"previous hour", strings.Replace(validTrack, "02:05:00Z", "01:59:59Z", 1), at, false},
		{"offset outside", strings.Replace(validTrack, "02:05:00Z", "02:05:00+11:00", 1), at, false},
		{"non hour fact time", validTrack, at.Add(time.Second), false},
		{"fractional fact time", validTrack, at.Add(time.Nanosecond), false},
		{"non UTC hour instant", validTrack, at.Add(30 * time.Minute).In(time.FixedZone("half", 30*60)), false},
		{"offset fact instant", validTrack, at.In(time.FixedZone("east", 11*3600)), true},
		{"zero fact time", validTrack, time.Time{}, false},
		{"object", `[]`, at, false}, {"null", `null`, at, false},
		{"ignored extras", strings.Replace(validTrack, `"device"`, `"id":12,"hour":{},"extra":null,"device"`, 1), at, true},
	}
	replacements := [][2]string{
		{`"device":"phone",`, ``}, {`"device"`, `"Device"`}, {`"lat"`, `"Lat"`}, {`"phone"`, `null`}, {`"phone"`, `""`}, {`"phone"`, `12`},
		{`"points":[`, `"other":[`},
		{`"t":"2026-10-12T02:05:00Z",`, ``}, {`"2026-10-12T02:05:00Z"`, `null`},
		{`"2026-10-12T02:05:00Z"`, `"2026-10-12"`},
		{`"lat":-33.8,`, ``}, {`-33.8`, `null`}, {`-33.8`, `"NaN"`},
		{`-33.8`, `1e999`}, {`-33.8`, `90.01`}, {`-33.8`, `-90.01`},
		{`"lon":151.2,`, ``}, {`151.2`, `null`}, {`151.2`, `180.01`}, {`151.2`, `-180.01`},
		{`151.2`, `1e999`}, {`14`, `-1`}, {`14`, `1e999`}, {`14`, `1.7976931348623157e308`},
		{`12}`, `-1}`}, {`12}`, `1e999}`},
	}
	for _, pair := range replacements {
		tests = append(tests, struct {
			name, payload string
			at            time.Time
			valid         bool
		}{pair[0] + " to " + pair[1], strings.Replace(validTrack, pair[0], pair[1], 1), at, false})
	}
	for _, points := range []string{`null`, `[]`, `[null]`, `[{}]`, `{}`, `"wrong"`} {
		tests = append(tests, struct {
			name, payload string
			at            time.Time
			valid         bool
		}{"points " + points, `{"device":"phone","points":` + points + `}`, at, false})
	}
	for _, optional := range []string{`null`, `0`} {
		tests = append(tests, struct {
			name, payload string
			at            time.Time
			valid         bool
		}{"optional " + optional, strings.ReplaceAll(strings.Replace(validTrack, `14`, optional, 1), `12}`, optional+`}`), at, true})
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			payload, err := facet.Canonicalize([]byte(tt.payload))
			if err != nil {
				t.Fatal(err)
			}
			f := facet.Facet{Type: "custom.track", FactTime: tt.at, Payload: payload}
			_, pts, err := parseTrack(f)
			if (err == nil) != tt.valid {
				t.Fatalf("valid = %v, want %v, error %v", err == nil, tt.valid, err)
			}
			if err != nil && (strings.Contains(err.Error(), "151.2") || strings.Contains(err.Error(), "phone")) {
				t.Fatal("error leaked payload")
			}
			if tt.valid && (len(pts) != 1 || layoutTrack(pts, hourHue).Count != 1) {
				t.Fatal("valid point lost")
			}
		})
	}
}

func TestTrackFiniteGeometry(t *testing.T) {
	t.Parallel()
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if finite(value) {
			t.Fatal("nonfinite value accepted")
		}
		for _, p := range []trackPoint{{Lat: value}, {Lon: value}, {Acc: value}} {
			if v := layoutTrack([]trackPoint{p}, hourHue); v.Count != 0 {
				t.Fatal("nonfinite geometry emitted")
			}
		}
	}
}
