package inspect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// lanesServer holds facets in two domains: two weights (health, the body family) and a
// gym visit (activity, the exercise family), months apart so none join a run.
func lanesServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	var seq int64
	admit := func(ref, typ, payload string, at time.Time) {
		t.Helper()
		f := seedFacet(t, ref, typ, payload)
		f.FactTime = at
		seq++
		if err := l.Admit(ctx, ledger.Watermark(seq-1), f, event(facet.EventAdmitted, f, seq)); err != nil {
			t.Fatalf("Admit %s: %v", ref, err)
		}
	}
	admit("scales:1", "health.weight", `{"kg": 80}`, time.Date(2026, 1, 10, 7, 0, 0, 0, time.UTC))
	admit("scales:2", "health.weight", `{"kg": 79}`, time.Date(2026, 4, 10, 7, 0, 0, 0, time.UTC))
	admit("gym:1", "activity.gym-visit", `{"at": "x"}`, time.Date(2026, 3, 1, 7, 0, 0, 0, time.UTC))
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Catalog: testCatalog(), Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(srv.Close)
	return srv
}

var (
	laneRe   = regexp.MustCompile(`<g class="domain([^"]*)" data-domain="([a-z]+)"`)
	markIDRe = regexp.MustCompile(`class="pt[^"]*" href="/facet\?id=([^"]+)"`)
	toggleRe = regexp.MustCompile(`data-domain="([a-z]+)">\s*<rect[^>]*/>\s*<a class="lane-toggle" href="([^"]+)"`)
)

// lanes reads the domain lanes off a band: id to its class flags.
func lanes(band string) map[string]string {
	out := map[string]string{}
	for _, m := range laneRe.FindAllStringSubmatch(band, -1) {
		out[m[2]] = m[1]
	}
	return out
}

func bandOf(page string) string {
	return page[:strings.Index(page, "compose — the canonical")]
}

func markIDs(band string) []string {
	var ids []string
	for _, m := range markIDRe.FindAllStringSubmatch(band, -1) {
		ids = append(ids, m[1])
	}
	sort.Strings(ids)
	return ids
}

// The band opens with one lane for each domain with a type: the seven in use, and
// none for the reserved heads. A lane is collapsed, its family rows are not drawn, and
// one with nothing in the window is greyed.
func TestTimeline_DomainLanesStartCollapsed(t *testing.T) {
	t.Parallel()
	srv := lanesServer(t)
	code, page := get(t, srv.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	band := bandOf(page)
	got := lanes(band)
	for _, d := range []string{"health", "activity", "place", "record", "mind", "org", "money"} {
		flags, ok := got[d]
		if !ok {
			t.Errorf("no lane for %s", d)
			continue
		}
		if strings.Contains(flags, "open") {
			t.Errorf("%s starts open", d)
		}
		if empty := strings.Contains(flags, "empty"); empty != (d != "health" && d != "activity") {
			t.Errorf("%s: empty = %v, want a greyed lane only where the window holds nothing", d, empty)
		}
	}
	for _, d := range []string{"social", "media", "work"} {
		if _, ok := got[d]; ok {
			t.Errorf("the reserved head %s has a lane before it has a type", d)
		}
	}
	if len(got) != 7 {
		t.Errorf("%d lanes, want 7: %v", len(got), got)
	}
	if strings.Contains(band, `class="rowlabel famlabel"`) {
		t.Error("a family row is drawn under a closed lane")
	}
	// Colour follows the domain: the weights take health's hue, the visit activity's.
	if !strings.Contains(band, `<path class="h7 hollow" d="`) || !strings.Contains(band, `<path class="h3" d="`) {
		t.Error("marks do not take their domain's hue")
	}
}

// Opening a lane shows its family rows and a lane per type; the address carries the
// state, the page's own links keep it, and closing returns the collapsed page. Every
// mark is on the page either way.
func TestTimeline_AnOpenedLaneRoundTripsThroughTheAddress(t *testing.T) {
	t.Parallel()
	srv := lanesServer(t)

	_, closed := get(t, srv.URL+"/")
	closedBand := bandOf(closed)
	var toggle string
	for _, m := range toggleRe.FindAllStringSubmatch(closedBand, -1) {
		if m[1] == "health" {
			toggle = m[2]
		}
	}
	if toggle != "/?open=health" {
		t.Fatalf("health's toggle is %q, want /?open=health", toggle)
	}

	code, opened := get(t, srv.URL+toggle)
	if code != http.StatusOK {
		t.Fatalf("GET %s = %d", toggle, code)
	}
	openBand := bandOf(opened)
	got := lanes(openBand)
	if !strings.Contains(got["health"], "open") || strings.Contains(got["activity"], "open") {
		t.Errorf("lanes %v: want health open and activity closed", got)
	}
	if !strings.Contains(openBand, `famlabel" x="26" y="`) || !strings.Contains(openBand, ">body</text>") {
		t.Error("the opened lane does not show its body family row")
	}
	if strings.Contains(openBand, ">exercise</text>") {
		t.Error("the closed activity lane shows its exercise row")
	}
	// The weights sit on the family row's sub-lane, off the lane's own line.
	if a, b := markIDs(closedBand), markIDs(openBand); strings.Join(a, ",") != strings.Join(b, ",") || len(a) != 3 {
		t.Errorf("marks closed %v, opened %v: want the same three", a, b)
	}
	if !strings.Contains(closed, "3 facets in this window") || !strings.Contains(opened, "3 facets in this window") {
		t.Error("the facets in this window differ between the closed and opened pages")
	}

	// The state rides every link of the page and the form, so a gesture keeps it.
	for _, want := range []string{`name="open" value="health"`, `id="nav-left" title="pan left (←)"`} {
		if !strings.Contains(opened, want) {
			t.Errorf("opened page lacks %q", want)
		}
	}
	for _, id := range []string{"nav-left", "show-events", "status-all"} {
		m := regexp.MustCompile(`href="([^"]+)" id="` + id + `"`).FindStringSubmatch(opened)
		if m == nil || !strings.Contains(m[1], "open=health") {
			t.Errorf("%s link does not carry the open lane: %v", id, m)
		}
	}
	// Closing it again is the lane's own link, back to the whole person.
	var closeLink string
	for _, m := range toggleRe.FindAllStringSubmatch(openBand, -1) {
		if m[1] == "health" {
			closeLink = m[2]
		}
	}
	if closeLink != "/?view=whole" {
		t.Errorf("health's close link is %q, want /?view=whole", closeLink)
	}
	// A name that is no lane is dropped.
	_, stale := get(t, srv.URL+"/?open=nope&open=social")
	for d, flags := range lanes(bandOf(stale)) {
		if strings.Contains(flags, "open") {
			t.Errorf("%s is open on a stale link", d)
		}
	}
	// Open all and close all are links.
	all := regexp.MustCompile(`href="([^"]+)" id="lanes-open"`).FindStringSubmatch(closed)
	if all == nil || strings.Count(all[1], "open=") != 7 {
		t.Errorf("open all = %v, want seven lanes", all)
	}
}

// A lane the type filter touches opens itself, with no toggle; the lanes it does not
// touch stay as they were.
func TestTimeline_ATypeFilterOpensItsLane(t *testing.T) {
	t.Parallel()
	srv := lanesServer(t)
	_, page := get(t, srv.URL+"/?type=health.weight")
	band := bandOf(page)
	got := lanes(band)
	if !strings.Contains(got["health"], "open") || strings.Contains(got["activity"], "open") {
		t.Errorf("lanes %v: want only health open", got)
	}
	if strings.Contains(band, `lane-toggle" href="/?open=health`) || !strings.Contains(band, `class="held"`) {
		t.Error("a lane the filter holds open still offers a toggle")
	}
	if n := len(markIDs(band)); n != 2 {
		t.Errorf("%d marks, want the two weights", n)
	}
}
