package inspect_test

import (
	"context"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// A bare date is a day in the reader's zone on every Workbench page (card 236), and
// the timeline and the select page hand a range to each other in it (card 237). The
// record is the card's own case: a pathology draw at 07:30 Sydney on 12 Oct, which is
// 20:30 UTC on the 11th, beside one at 23:59 on the 11th Sydney time.

const sydney = "Australia/Sydney"

var zoneRecord = []struct {
	ref, typ string
	at       time.Time
}{
	{"draw", "health.weight", time.Date(2026, 10, 11, 20, 30, 0, 0, time.UTC)},   // 07:30 on 12 Oct in Sydney
	{"before", "health.weight", time.Date(2026, 10, 11, 12, 59, 0, 0, time.UTC)}, // 23:59 on 11 Oct in Sydney
	{"bp", "health.blood-pressure", time.Date(2026, 10, 12, 1, 0, 0, 0, time.UTC)},
	{"hr-alert", "health.heart-rate.alert", time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)},
	{"hr-day", "health.heart-rate.day", time.Date(2026, 10, 12, 3, 0, 0, 0, time.UTC)},
}

func zoneServer(t *testing.T, zone string) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	for i, p := range zoneRecord {
		f := seedFacetNoT("zone:"+p.ref, p.typ, `{"n": `+strconv.Itoa(i)+`}`)
		f.FactTime = p.at
		if err := l.Admit(ctx, ledger.Watermark(i), f, event(facet.EventAdmitted, f, int64(i+1))); err != nil {
			t.Fatalf("Admit %s: %v", p.ref, err)
		}
	}
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{
		Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, MaxReadBytes: 512 << 10,
		Selections: selection.NewStore(a), Zone: zone,
	}))
	t.Cleanup(srv.Close)
	return srv
}

func zid(ref string) string { return facet.NewID("zone:" + ref) }

var (
	zoneFieldRE = regexp.MustCompile(`<input name="zone" id="zone" value="([^"]*)"([^>]*)>`)
	windowRE    = regexp.MustCompile(`const from =\s*(-?\d+)\s*,\s*to =\s*(-?\d+)\s*;`)
)

// window is the timeline's drawn window, as instants.
func window(t *testing.T, body string) (time.Time, time.Time) {
	t.Helper()
	m := windowRE.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no window in the page")
	}
	lo, _ := strconv.ParseInt(m[1], 10, 64)
	hi, _ := strconv.ParseInt(m[2], 10, 64)
	return time.UnixMilli(lo).UTC(), time.UnixMilli(hi).UTC()
}

// The card's own case: a selection built "from 12 Oct" in Sydney holds the draw at
// 07:30 Sydney and not the facet at 23:59 the night before; the same date in UTC holds
// neither. The program carries the instants and says which zone read them.
func TestSelect_AFromDateIsADayInTheReadersZone(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	cases := []struct {
		name, zone string
		matched    int
		wantFrom   string
	}{
		{"sydney", sydney, 1, "2026-10-11T13:00:00.000000000Z"},
		{"utc", "UTC", 0, "2026-10-12T00:00:00.000000000Z"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			code, body, _ := fetch(t, srv.URL+"/select?type=health.weight&from=2026-10-12&zone="+c.zone)
			if code != 200 {
				t.Fatalf("GET = %d\n%s", code, body)
			}
			if got := number(t, matchedRE, body); got != c.matched {
				t.Errorf("matched %d, want %d", got, c.matched)
			}
			if c.matched == 1 {
				if all, _ := boxes(body); len(all) != 1 || all[0] != zid("draw") {
					t.Errorf("boxes %v, want the draw alone", all)
				}
			}
			prog := program(t, body)
			if !strings.Contains(prog, `At >= "`+c.wantFrom+`"`) {
				t.Errorf("the program lacks the bound %s:\n%s", c.wantFrom, prog)
			}
			if want := "% dates read in " + c.zone + "\n"; !strings.Contains(prog, want) {
				t.Errorf("the program lacks the line %q:\n%s", want, prog)
			}
			// It parses back to the same filter, and the zone with it.
			f, err := selection.Parse(prog)
			if err != nil || f.From != c.wantFrom {
				t.Errorf("Parse = %+v %v", f, err)
			}
			if selection.ZoneOf(prog) != c.zone {
				t.Errorf("ZoneOf = %q, want %q", selection.ZoneOf(prog), c.zone)
			}
			// The form says the zone, and the chip reads the date the person typed.
			if m := zoneFieldRE.FindStringSubmatch(body); m == nil || m[1] != c.zone || strings.Contains(m[2], "data-fill") {
				t.Errorf("zone field = %v, want %s named by the request", m, c.zone)
			}
			if chips := removeRE.FindAllStringSubmatch(body, -1); len(chips) < 2 || chips[0][1] != "from 2026-10-12" {
				t.Errorf("chips = %v, want from 2026-10-12 first", chips)
			}
		})
	}
}

// A bound the zone's midnight writes back as a date; any other instant stays one.
func TestSelect_ABoundIsADateOnlyAtMidnightInTheZone(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	_, midnight, _ := fetch(t, srv.URL+"/select?from=2026-10-11T13:00:00.000000000Z&zone="+sydney)
	if chips := removeRE.FindAllStringSubmatch(midnight, -1); len(chips) != 1 || chips[0][1] != "from 2026-10-12" {
		t.Errorf("a Sydney midnight is not a date: %v", chips)
	}
	_, off, _ := fetch(t, srv.URL+"/select?from=2026-10-11T20:30:00.000000000Z&zone="+sydney)
	if chips := removeRE.FindAllStringSubmatch(off, -1); len(chips) != 1 || chips[0][1] != "from 2026-10-11T20:30:00.000000000Z" {
		t.Errorf("an instant off midnight is not kept whole: %v", chips)
	}
	// The address the page redirects to carries the zone, so the date keeps its meaning.
	_, _, landed := fetch(t, srv.URL+"/select?from=2026-10-12&zone="+sydney+"&shown=x")
	if landed.Query().Get("zone") != sydney || landed.Query().Get("from") != "2026-10-12" {
		t.Errorf("landed at %s, want the date and the zone kept", landed)
	}
}

// A request without a zone reads in the zone the daemon is configured with; with no
// configured zone, in UTC. The field says which, and is left to the page's script
// only while nothing on the page has been read yet.
func TestSelect_WithoutAZoneItReadsInTheDaemonsZone(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name, zone string
		matched    int
	}{{"configured", sydney, 1}, {"unconfigured", "", 0}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			srv := zoneServer(t, c.zone)
			_, body, _ := fetch(t, srv.URL+"/select?type=health.weight&from=2026-10-12")
			if got := number(t, matchedRE, body); got != c.matched {
				t.Errorf("matched %d, want %d", got, c.matched)
			}
			want := c.zone
			if want == "" {
				want = "UTC"
			}
			if m := zoneFieldRE.FindStringSubmatch(body); m == nil || m[1] != want || strings.Contains(m[2], "data-fill") {
				t.Errorf("a dated page's zone field = %v, want %s and no script fill", m, want)
			}
			_, fresh, _ := fetch(t, srv.URL+"/select")
			if m := zoneFieldRE.FindStringSubmatch(fresh); m == nil || m[1] != want || !strings.Contains(m[2], "data-fill") {
				t.Errorf("an empty page's zone field = %v, want %s open to the browser's zone", m, want)
			}
		})
	}
}

// An unknown zone, and Local, are refused with the zone named, on every page that
// reads a date.
func TestPages_RefuseAZoneTheyCannotRead(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	for _, page := range []string{"/select?from=2026-10-12", "/?from=2026-10-12", "/projection?type=health.weight", "/location"} {
		for _, zone := range []string{"Mars/Olympus", "Local"} {
			sep := "&"
			if !strings.Contains(page, "?") {
				sep = "?"
			}
			code, body, _ := fetch(t, srv.URL+page+sep+"zone="+url.QueryEscape(zone))
			if code != 400 || !strings.Contains(body, zone) {
				t.Errorf("%s zone=%s = %d %q, want 400 naming the zone", page, zone, code, strings.TrimSpace(body))
			}
		}
	}
}

// The projection page's rows view reads its days in the zone too.
func TestProjection_RowsReadTheirDaysInTheReadersZone(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	rows := regexp.MustCompile(`<strong>health.weight</strong>, served: (\d+) row`)
	for zone, want := range map[string]string{sydney: "1", "UTC": "0"} {
		_, body := get(t, srv.URL+"/projection?type=health.weight&from=2026-10-12&to=2026-10-12&zone="+zone)
		if m := rows.FindStringSubmatch(body); m == nil || m[1] != want {
			t.Errorf("zone %s: rows = %v, want %s", zone, m, want)
		}
		if m := zoneFieldRE.FindStringSubmatch(body); m == nil || m[1] != zone {
			t.Errorf("zone %s: the page's zone field = %v", zone, m)
		}
	}
}

// The timeline reads a bare from as the start of that day in the zone, and a bare to
// as the end of it.
func TestTimeline_ABareDateIsADayInTheReadersZone(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	lo, hi := time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC), time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC)
	for _, c := range []struct{ name, zone, query string }{
		{"named", "", "&zone=" + sydney},
		{"the daemon's", sydney, ""},
	} {
		srv := srv
		if c.zone != "" {
			srv = zoneServer(t, c.zone)
		}
		_, body := get(t, srv.URL+"/?from=2026-10-12&to=2026-10-12"+c.query)
		if f, to := window(t, body); !f.Equal(lo) || !to.Equal(hi) {
			t.Errorf("%s: window %s to %s, want %s to %s", c.name, f, to, lo, hi)
		}
	}
	_, utc := get(t, srv.URL+"/?from=2026-10-12&to=2026-10-12")
	if f, to := window(t, utc); !f.Equal(time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("without a zone: window %s to %s, want the UTC day", f, to)
	}
}

// A saved selection keeps its instants and says which zone its dates were read in:
// from the program's comment when it has one, UTC when it has none.
func TestSelectSaved_NamesTheZoneItsDatesWereReadIn(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	_, page, _ := fetch(t, srv.URL+"/select?type=health.weight&from=2026-10-12&zone="+sydney)
	_, body := save(t, srv, program(t, page))
	if !strings.Contains(body, "Its dates were read in "+sydney+".") {
		t.Errorf("the saved page does not name its zone:\n%s", body)
	}
	if m := zoneFieldRE.FindStringSubmatch(body); m == nil || m[1] != sydney {
		t.Errorf("the saved page's zone field = %v", m)
	}
	if chips := removeRE.FindAllStringSubmatch(body, -1); len(chips) < 1 || chips[0][1] != "from 2026-10-12" {
		t.Errorf("the saved page reads its bound as %v, want the Sydney date", chips)
	}
	if prog := program(t, body); !strings.Contains(prog, `At >= "2026-10-11T13:00:00.000000000Z"`) {
		t.Errorf("the saved instant changed:\n%s", prog)
	}

	// Saved before the comment: the same kind of program with no zone line reads UTC.
	old, err := selection.Filter{From: "2026-10-12T00:00:00.000000000Z", Types: []string{"health.weight"}}.Program()
	if err != nil {
		t.Fatal(err)
	}
	_, oldBody := save(t, srv, old)
	if !strings.Contains(oldBody, "Its dates were read in UTC.") {
		t.Errorf("a selection with no zone comment does not say UTC:\n%s", oldBody)
	}
	if chips := removeRE.FindAllStringSubmatch(oldBody, -1); len(chips) < 1 || chips[0][1] != "from 2026-10-12" {
		t.Errorf("an old selection reads its bound as %v, want the UTC date", chips)
	}
}
