package inspect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Two hours of positions on 2026-10-12 (UTC) from the phone "tokay", the shape the
// anam_owntracks importer writes (card 130), and one hour the day before, so the
// default window has something to leave out. A fix with no accuracy and a fix a
// kilometre away are both drawn.
const (
	hourA = `{"id":"loc:tokay:2026-10-12T02:00:00Z","hour":"2026-10-12T02:00:00Z","device":"tokay","points":[
	 {"t":"2026-10-12T02:05:00Z","lat":-33.8688,"lon":151.2093,"acc":14,"vel":0},
	 {"t":"2026-10-12T02:20:00Z","lat":-33.8700,"lon":151.2100,"acc":9,"vel":12},
	 {"t":"2026-10-12T02:50:00Z","lat":-33.8750,"lon":151.2150}]}`
	hourB = `{"id":"loc:tokay:2026-10-12T03:00:00Z","hour":"2026-10-12T03:00:00Z","device":"tokay","points":[
	 {"t":"2026-10-12T03:10:00Z","lat":-33.8790,"lon":151.2200,"acc":20,"vel":30}]}`
	hourPrev = `{"id":"loc:tokay:2026-10-11T22:00:00Z","hour":"2026-10-11T22:00:00Z","device":"tokay","points":[
	 {"t":"2026-10-11T22:30:00Z","lat":-33.8600,"lon":151.2000,"acc":10}]}`
	hourOther = `{"id":"loc:other:2026-10-12T02:00:00Z","hour":"2026-10-12T02:00:00Z","device":"other","points":[
	 {"t":"2026-10-12T02:15:00Z","lat":-33.9000,"lon":151.1000,"acc":50}]}`
)

// locationServer seeds the location hours above beside the usual two facets and
// returns the server and hour A's facet id.
func locationServer(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })

	var seq int64
	admit := func(ref, ftype, payload string, at time.Time) facet.Facet {
		f := seedFacet(t, ref, ftype, payload)
		f.FactTime = at
		seq++
		if err := l.Admit(ctx, ledger.Watermark(seq-1), f, event(facet.EventAdmitted, f, seq)); err != nil {
			t.Fatalf("Admit %s: %v", ref, err)
		}
		return f
	}
	admit("bp-csv:2026-01-24T06:30:00", "health.blood-pressure", `{"systolic": 118}`, time.Date(2026, 7, 5, 6, 30, 0, 0, time.UTC))
	a := admit("owntracks:loc:tokay:2026-10-12T02:00:00Z", "place.track", hourA, time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC))
	admit("owntracks:loc:tokay:2026-10-12T03:00:00Z", "place.track", hourB, time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC))
	admit("owntracks:loc:tokay:2026-10-11T22:00:00Z", "place.track", hourPrev, time.Date(2026, 10, 11, 22, 0, 0, 0, time.UTC))
	admit("owntracks:loc:other:2026-10-12T02:00:00Z", "place.track", hourOther, time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC))

	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Definitions: trackDefinitions(t)}))
	t.Cleanup(srv.Close)
	return srv, a.ID
}

// trackDefinitions is the registry the owntracks importer leaves the daemon with: a
// place.track Definition declaring its family and kind (card 147), which is what
// places the hours on the band.
func trackDefinitions(t *testing.T) func() *def.Registry {
	t.Helper()
	d, err := def.Parse([]byte(`{"meta": {"facet_type": "place.track", "definition_version": 1, "classification": "Sensitive",
	  "validation_mode": "strict", "source_schema": "owntracks/hour", "kind": "measure", "family": "location",
	  "time_semantics": {"fact_time": {"pointer": "/hour", "layout": "2006-01-02T15:04:05Z07:00", "zone": "UTC"}},
	  "identity_key": ["/id"]},
	 "schema": {"$schema": "https://json-schema.org/draft/2020-12/schema", "type": "object",
	  "required": ["id", "hour"], "additionalProperties": false, "properties": {"id": {"type": "string", "minLength": 1}, "hour": {"type": "string", "minLength": 1}}}}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	reg, err := def.NewRegistry(d)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	return func() *def.Registry { return reg }
}

// A place.track hour draws in the location row as a measure (cards 133, 147): the
// legend lists it under location, not "other".
func TestTimeline_LocationHoursSitInTheLocationRow(t *testing.T) {
	t.Parallel()
	srv, _ := locationServer(t)
	_, body := get(t, srv.URL+"/")
	if !strings.Contains(body, `class="fam"`) {
		t.Fatal("no legend")
	}
	if strings.Contains(body, ">other<") {
		t.Error("an 'other' row drawn: place.track has no style")
	}
	if !strings.Contains(body, `value="place.track" checked`) {
		t.Error("legend does not list place.track")
	}
}

// The facet page draws the hour above the payload: the fixes, their accuracy
// rings, the scale bar, and the first and last marks.
func TestFacetPage_DrawsTheHourAsATrack(t *testing.T) {
	t.Parallel()
	srv, id := locationServer(t)
	code, body := get(t, srv.URL+"/facet?id="+id)
	if code != http.StatusOK {
		t.Fatalf("GET /facet = %d", code)
	}
	for _, want := range []string{`class="track"`, `class="line"`, `class="first"`, `class="last"`, `class="scale"`,
		"3 fixes over 1 hour", "2026-10-12 02:05Z to 02:50Z", "±14 m", "12 km/h", "device tokay"} {
		if !strings.Contains(body, want) {
			t.Errorf("facet page missing %q", want)
		}
	}
	// Two fixes have an accuracy, one does not: two rings.
	if n := strings.Count(body, `class="ring"`); n != 2 {
		t.Errorf("%d accuracy rings, want 2", n)
	}
	if !strings.Contains(body, `href="/facet?id=`+id+`"`) {
		t.Error("fixes do not link to their hour")
	}
}

// /location defaults to the newest UTC day with positions, filters by device, and
// takes the timeline's window.
func TestLocation_DrawsTheNewestDayAndNarrows(t *testing.T) {
	t.Parallel()
	srv, _ := locationServer(t)

	code, body := get(t, srv.URL+"/location")
	if code != http.StatusOK {
		t.Fatalf("GET /location = %d", code)
	}
	for _, want := range []string{"5 fixes over 3 hours", "2026-10-12 02:05Z to 03:10Z", `value="2026-10-12T00:00:00Z"`, "<option>tokay</option>", "<option>other</option>"} {
		if !strings.Contains(body, want) {
			t.Errorf("/location missing %q", want)
		}
	}
	if strings.Contains(body, "2026-10-11 22:30Z") {
		t.Error("the day before leaked into the newest day")
	}

	_, tokay := get(t, srv.URL+"/location?device=tokay")
	if !strings.Contains(tokay, "4 fixes over 2 hours") {
		t.Error("device filter did not narrow to tokay's hours")
	}

	_, window := get(t, srv.URL+"/location?from=2026-10-11&to=2026-10-11&device=tokay")
	if !strings.Contains(window, "1 fix over 1 hour") || !strings.Contains(window, "2026-10-11 22:30Z") {
		t.Error("window did not narrow to the day before")
	}

	if code, _ := get(t, srv.URL+"/location?from=2026-10-13&to=2026-10-12"); code != http.StatusBadRequest {
		t.Errorf("empty window = %d, want 400", code)
	}
}

// A ledger with no positions says so rather than drawing nothing.
func TestLocation_EmptyRecordSaysSo(t *testing.T) {
	t.Parallel()
	srv, _, _ := seededServer(t)
	code, body := get(t, srv.URL+"/location")
	if code != http.StatusOK || !strings.Contains(body, "no place.track facets are live") {
		t.Errorf("GET /location = %d, body lacks the empty notice", code)
	}
}
