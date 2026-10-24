package inspect_test

import (
	"html"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

// The hand-off between the timeline and the select page (card 237), over a real
// ledger: the record and the server are zone_test.go's.

// link is the href of the anchor with the id, unescaped; "" when there is none.
func link(body, id string) string {
	m := regexp.MustCompile(`<a [^>]*href="([^"]+)"[^>]* id="` + id + `"`).FindStringSubmatch(body)
	if m == nil {
		m = regexp.MustCompile(`<a href="([^"]+)" id="` + id + `"`).FindStringSubmatch(body)
	}
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

// Card 237: the timeline hands its window to the select page, and the select page its
// filter to the timeline, in the same zone, so a Sydney day stays a Sydney day.
func TestHandoff_ASydneyDayIsASydneyDayBothWays(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	lo, hi := time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC), time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC)

	// Select to timeline.
	_, sel, _ := fetch(t, srv.URL+"/select?type=health.weight&from=2026-10-12&to=2026-10-12&zone="+sydney)
	toTimeline := link(sel, "show-on-timeline")
	if want := "/?from=2026-10-12&to=2026-10-12&type=health.weight&zone=Australia%2FSydney"; toTimeline != want {
		t.Fatalf("show on timeline = %s, want %s", toTimeline, want)
	}
	_, tl := get(t, srv.URL+toTimeline)
	if f, to := window(t, tl); !f.Equal(lo) || !to.Equal(hi) {
		t.Errorf("the timeline window is %s to %s, want the Sydney day %s to %s", f, to, lo, hi)
	}

	// Timeline to select, and back to the program the first page showed.
	back := link(tl, "select-range")
	if want := "/select?from=2026-10-12&to=2026-10-12&type=health.weight&zone=Australia%2FSydney"; back != want {
		t.Fatalf("select in this range = %s, want %s", back, want)
	}
	_, again, _ := fetch(t, srv.URL+back)
	if program(t, again) != program(t, sel) {
		t.Errorf("the round trip changed the program:\n%s\nwas\n%s", program(t, again), program(t, sel))
	}
}

// A window off the zone's midnights travels as instants, and arrives as the same
// window.
func TestHandoff_AWindowOffMidnightTravelsAsInstants(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	_, tl := get(t, srv.URL+"/?from=2026-10-11T20:00:00Z&to=2026-10-12T02:00:00Z&zone="+sydney)
	toSelect := link(tl, "select-range")
	if want := "/select?from=2026-10-11T20%3A00%3A00.000000000Z&to=2026-10-12T02%3A00%3A00.000000000Z&zone=Australia%2FSydney"; toSelect != want {
		t.Fatalf("select in this range = %s, want %s", toSelect, want)
	}
	_, sel, _ := fetch(t, srv.URL+toSelect)
	prog := program(t, sel)
	for _, want := range []string{`At >= "2026-10-11T20:00:00.000000000Z"`, `At < "2026-10-12T02:00:00.000000000Z"`, "% dates read in " + sydney} {
		if !strings.Contains(prog, want) {
			t.Errorf("the program lacks %s:\n%s", want, prog)
		}
	}
	_, tl2 := get(t, srv.URL+link(sel, "show-on-timeline"))
	f, to := window(t, tl2)
	if !f.Equal(time.Date(2026, 10, 11, 20, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 10, 12, 2, 0, 0, 0, time.UTC)) {
		t.Errorf("the window came back as %s to %s", f, to)
	}
}

// The unzoomed timeline has no range to hand on.
func TestHandoff_AnUnzoomedTimelineOffersNoRange(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	_, body := get(t, srv.URL+"/")
	if strings.Contains(body, `id="select-range"`) {
		t.Error("the unzoomed timeline offers to select in a range")
	}
	_, zoomed := get(t, srv.URL+"/?from=2026-10-12&to=2026-10-12")
	if link(zoomed, "select-range") == "" {
		t.Error("the zoomed timeline offers no range")
	}
}

// The type filter travels both ways. A prefix on the select page names the types it
// covers on the timeline, which filters by exact type.
func TestHandoff_CarriesTheTypeFilter(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	_, sel, _ := fetch(t, srv.URL+"/select?type=health.heart-rate.")
	want := "/?type=health.heart-rate.alert&type=health.heart-rate.day"
	if got := link(sel, "show-on-timeline"); got != want {
		t.Errorf("show on timeline = %s, want %s", got, want)
	}
	_, tl := get(t, srv.URL+"/?from=2026-10-12&to=2026-10-12&zone="+sydney+"&type=health.blood-pressure&type=health.weight")
	got := link(tl, "select-range")
	if want := "/select?from=2026-10-12&to=2026-10-12&type=health.blood-pressure&type=health.weight&zone=Australia%2FSydney"; got != want {
		t.Errorf("select in this range = %s, want %s", got, want)
	}
	// A filter with nothing to say opens the whole person.
	_, bare, _ := fetch(t, srv.URL+"/select")
	if got := link(bare, "show-on-timeline"); got != "/?view=whole" {
		t.Errorf("an empty filter shows on the timeline at %s", got)
	}
}

// A saved selection opens on the timeline at its window, drawn as it is today.
func TestHandoff_ASavedSelectionIsDrawnAtItsWindow(t *testing.T) {
	t.Parallel()
	srv := zoneServer(t, "")
	_, page, _ := fetch(t, srv.URL+"/select?type=health.weight&from=2026-10-12&to=2026-10-12&zone="+sydney)
	code, saved := save(t, srv, program(t, page))
	want := "/?from=2026-10-12&sel=" + code + "&to=2026-10-12&zone=Australia%2FSydney&type=health.weight"
	got := link(saved, "show-on-timeline")
	u, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	wu, _ := url.Parse(want)
	if u.Path != "/" || u.Query().Encode() != wu.Query().Encode() {
		t.Fatalf("show on timeline = %s, want %s", got, want)
	}
	_, tl := get(t, srv.URL+got)
	if !strings.Contains(tl, "selection sel:"+code+", 1 facet") {
		t.Errorf("the timeline does not draw the saved selection:\n%s", tl)
	}
	if f, to := window(t, tl); !f.Equal(time.Date(2026, 10, 11, 13, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 10, 12, 13, 0, 0, 0, time.UTC)) {
		t.Errorf("window %s to %s, want the Sydney day", f, to)
	}
}
