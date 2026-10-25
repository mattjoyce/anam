package inspect_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// countingLedger counts the dead-version reads the band makes, so a test can say
// a page never asked for them (card 135).
type countingLedger struct {
	*ledger.Ledger
	versionReads atomic.Int64
}

func (c *countingLedger) Version(ctx context.Context, id string, v int) (facet.Facet, error) {
	c.versionReads.Add(1)
	return c.Ledger.Version(ctx, id, v)
}

func (c *countingLedger) Versions(ctx context.Context, refs []ledger.VersionRef) ([]facet.Facet, error) {
	c.versionReads.Add(1)
	return c.Ledger.Versions(ctx, refs)
}

// denseServer seeds forty heart-rate readings ten minutes apart on 2026-07-05, a
// blood pressure reading in 2024 to stretch the whole-person window over years,
// 1,500 daily weights, and one tombstoned facet. It returns the server and the
// counting ledger behind it.
func denseServer(t *testing.T) (*httptest.Server, *countingLedger) {
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
	admit("bp-csv:2024-01-01T06:30:00", "health.blood-pressure", `{"systolic": 118}`, time.Date(2024, 1, 1, 6, 30, 0, 0, time.UTC))
	for i := 0; i < 40; i++ {
		at := time.Date(2026, 7, 5, 6, 0, 0, 0, time.UTC).Add(time.Duration(i) * 10 * time.Minute)
		admit(fmt.Sprintf("health-db:hr:%d", i), "health.heart-rate", fmt.Sprintf(`{"resting": %d}`, 50+i), at)
	}
	// 1,500 daily weights over 2025 to 2029 tip the strip past what it draws.
	for i := 0; i < 1500; i++ {
		at := time.Date(2025, 1, 1, 7, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		admit(fmt.Sprintf("scale:%d", i), "health.weight", fmt.Sprintf(`{"kg": %d}`, 80), at)
	}
	gone := admit("health-db:hr:gone", "health.heart-rate", `{"resting": 99}`, time.Date(2026, 7, 6, 6, 0, 0, 0, time.UTC))
	seq++
	if err := l.Mark(ctx, ledger.Watermark(seq-1), event(facet.EventTombstoned, gone, seq)); err != nil {
		t.Fatalf("Mark tombstone: %v", err)
	}

	cl := &countingLedger{Ledger: l}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: cl, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(srv.Close)
	return srv, cl
}

// Over years, forty readings in one morning are a run: two marks, a line and a
// count, whose link is the morning. Over the day they are forty marks, and the
// day's page is the larger one: the page tracks the screen, not the record.
func TestTimeline_DenseMarksDrawAsARunUntilZoomedIn(t *testing.T) {
	t.Parallel()
	srv, _ := denseServer(t)

	code, whole := get(t, srv.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("GET / = %d", code)
	}
	band := whole[:strings.Index(whole, "compose — the canonical")]
	// Health is a closed lane, so its two types share one line: the morning's readings
	// fall inside the weights' stretch and join its run (card 245).
	if n := strings.Count(band, `class="run"`); n != 1 {
		t.Fatalf("%d runs on the collapsed whole-person band, want 1 (all of health)", n)
	}
	if n := strings.Count(band, `/facet?id=`); n != 1 {
		t.Errorf("%d marks on the whole-person band, want the 2024 reading alone", n)
	}
	for _, want := range []string{"1540 × health (2 types)", "1541 facets in this window, 1 run", "click to zoom in", `<text x="`} {
		if !strings.Contains(whole, want) {
			t.Errorf("collapsed whole-person band missing %q", want)
		}
	}
	// Opened, each type has a lane of its own: the morning and the weights are two runs.
	_, opened := get(t, srv.URL+"/?open=health")
	openBand := opened[:strings.Index(opened, "compose — the canonical")]
	if n := strings.Count(openBand, `class="run"`); n != 2 {
		t.Fatalf("%d runs on the opened band, want 2 (the morning and the weights)", n)
	}
	for _, want := range []string{"40 × health.heart-rate", "1500 × health.weight", "1541 facets in this window, 2 runs"} {
		if !strings.Contains(opened, want) {
			t.Errorf("opened band missing %q", want)
		}
	}
	// The strip, past 1,500 live facets, tallies by type instead of a square each.
	strip := whole[len(band):]
	strip = strip[:strings.Index(strip, "<script>")]
	if strings.Count(strip, `/facet?id=`) != 0 || !strings.Contains(strip, "1541 live facets composed") ||
		!strings.Contains(strip, `href="/?type=health.weight"><span class="sw h7"></span>health.weight <span class="dim">(1500)</span>`) {
		t.Errorf("the strip drew a square each instead of tallying by type: %s", strip[:min(len(strip), 1500)])
	}
	runAt := strings.Index(opened, `<title>40 × health.heart-rate`)
	runAt = strings.LastIndex(opened[:runAt], `class="run"`)
	run := opened[runAt : runAt+strings.Index(opened[runAt:], ">")]
	if !strings.Contains(run, "from=2026-07-05T") || !strings.Contains(run, "to=2026-07-05T") {
		t.Errorf("the run's link is not its own morning: %s", run)
	}

	_, day := get(t, srv.URL+"/?from=2026-07-05&to=2026-07-05")
	if strings.Contains(day, `class="run"`) {
		t.Error("a run drawn when the marks are ten minutes apart over a day")
	}
	if n := strings.Count(day, `class="pt"`); n != 41 {
		t.Errorf("%d marks over the day, want 40 readings and a weight", n)
	}
	if !strings.Contains(day, "41 live facets composed from the span, in fact-time order") {
		t.Error("the day's strip is not drawn square by square")
	}
	if len(whole) >= len(day) {
		t.Errorf("whole-person page (%d bytes) is not smaller than the day's (%d)", len(whole), len(day))
	}
}

// A bare "/" draws the live alone and never reads a dead version from the ledger;
// the legend says how many dead are hidden, with a link to all of them.
func TestTimeline_LiveByDefaultLeavesTheDeadUnread(t *testing.T) {
	t.Parallel()
	srv, cl := denseServer(t)
	get(t, srv.URL+"/location") // warm the fold's cache: its own batch read is not under test
	cl.versionReads.Store(0)

	_, body := get(t, srv.URL+"/")
	if strings.Contains(body, `class="pt dead"`) {
		t.Error("a dead mark drawn by default")
	}
	if n := cl.versionReads.Load(); n != 0 {
		t.Errorf("%d version reads for a live-only page, want 0", n)
	}
	for _, want := range []string{`id="dead-hidden">1 dead hidden</a>`, `id="status-all">all</a>`, "the dead on request"} {
		if !strings.Contains(body, want) {
			t.Errorf("default page missing %q", want)
		}
	}

	_, all := get(t, srv.URL+"/?status=all")
	if !strings.Contains(all, `class="pt dead"`) {
		t.Error("status=all did not draw the dead")
	}
	if n := cl.versionReads.Load(); n != 1 {
		t.Errorf("%d version reads for status=all, want 1 batch", n)
	}
	if strings.Contains(all, "dead hidden") {
		t.Error("status=all still says the dead are hidden")
	}

	// The old spelling still reads, and is not what links carry.
	if code, _ := get(t, srv.URL+"/?status=live"); code != http.StatusOK {
		t.Errorf("GET /?status=live = %d", code)
	}
}

// "none" unchecks every type: nothing on the band or the strip, the preset shown
// as the current one, and the view carried on the links and the form.
func TestTimeline_NoneUnchecksEveryType(t *testing.T) {
	t.Parallel()
	srv, bpID, _ := seededServer(t)

	_, body := get(t, srv.URL+"/")
	if !strings.Contains(body, `href="/?type=none" id="show-none">none</a>`) {
		t.Error("no none preset beside all types")
	}

	code, none := get(t, srv.URL+"/?type=none")
	if code != http.StatusOK {
		t.Fatalf("GET /?type=none = %d", code)
	}
	if strings.Contains(none, "/facet?id="+bpID) {
		t.Error("a facet drawn under type=none")
	}
	for _, want := range []string{`id="show-none">none</span>`, `name="type" value="none"`, "no types shown", "no live facets in this span"} {
		if !strings.Contains(none, want) {
			t.Errorf("none view missing %q", want)
		}
	}
	at := strings.Index(none, `id="nav-left"`)
	left := none[strings.LastIndex(none[:at], "<a href="):at]
	if !strings.Contains(left, "type=none") {
		t.Errorf("pan-left link dropped none: %s", left)
	}
	// Legend boxes are all unticked, so one tick shows one type alone.
	if strings.Contains(none, `value="health.blood-pressure" checked`) {
		t.Error("a type still ticked under none")
	}
}

// A view is a URL, so "make this my default" keeps its query in a cookie and a bare
// "/" opens it; "forget" returns "/" to the whole person, which stays reachable.
func TestTimeline_DefaultViewIsACookieOverTheQuery(t *testing.T) {
	t.Parallel()
	srv, bpID, _ := seededServer(t)
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	do := func(req *http.Request) (*http.Response, string) {
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", req.Method, req.URL, err)
		}
		defer func() { _ = resp.Body.Close() }()
		body, _ := io.ReadAll(resp.Body)
		return resp, string(body)
	}

	view := "from=2026-07-01T00:00:00Z&to=2026-07-10T00:00:00Z&type=health.blood-pressure"
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/?"+view, nil)
	_, page := do(req)
	if !strings.Contains(page, `id="default-set"`) || strings.Contains(page, `id="default-forget"`) {
		t.Error("a view without a default does not offer to become one")
	}
	if !strings.Contains(page, `id="whole"`) {
		t.Error("a zoomed view has no way back to the whole person")
	}

	form := url.Values{"view": {view}}
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/view", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ := do(req)
	if resp.StatusCode != http.StatusSeeOther || resp.Header.Get("Location") != "/" {
		t.Fatalf("POST /view = %d %s, want 303 to /", resp.StatusCode, resp.Header.Get("Location"))
	}
	cookies := resp.Cookies()
	if len(cookies) != 1 || cookies[0].Name != "anam_view" || !cookies[0].HttpOnly {
		t.Fatalf("cookies = %v, want one HttpOnly anam_view", cookies)
	}
	cookie := cookies[0]

	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	req.AddCookie(cookie)
	resp, _ = do(req)
	loc := resp.Header.Get("Location")
	if resp.StatusCode != http.StatusSeeOther || !strings.HasPrefix(loc, "/?") ||
		!strings.Contains(loc, "type=health.blood-pressure") || !strings.Contains(loc, "from=2026-07-01") {
		t.Fatalf("GET / with a default = %d %s, want 303 to the view", resp.StatusCode, loc)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+loc, nil)
	req.AddCookie(cookie)
	resp, page = do(req)
	if resp.StatusCode != http.StatusOK || !strings.Contains(page, "/facet?id="+bpID) {
		t.Fatalf("the default view = %d, or does not draw the facet", resp.StatusCode)
	}
	for _, want := range []string{"this view is your default", `id="default-forget"`, `href="/?view=whole" id="whole"`} {
		if !strings.Contains(page, want) {
			t.Errorf("default view missing %q", want)
		}
	}

	// The whole person is still a page, and an explicit query never redirects.
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/?view=whole", nil)
	req.AddCookie(cookie)
	resp, page = do(req)
	if resp.StatusCode != http.StatusOK || strings.Contains(page, "this view is your default") {
		t.Errorf("GET /?view=whole with a default = %d, or claims to be the default", resp.StatusCode)
	}

	// Forgetting clears the cookie and "/" is the whole person again.
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/view", strings.NewReader("view="))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(cookie)
	resp, _ = do(req)
	if cs := resp.Cookies(); len(cs) != 1 || cs[0].MaxAge >= 0 {
		t.Errorf("forget did not expire the cookie: %v", cs)
	}
	req, _ = http.NewRequest(http.MethodGet, srv.URL+"/", nil)
	resp, _ = do(req)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET / without a default = %d", resp.StatusCode)
	}

	// A stored view carries only the view's own keys, nothing a page can be sent to.
	req, _ = http.NewRequest(http.MethodPost, srv.URL+"/view", strings.NewReader("view=status%3Dall%26next%3Dhttps%3A%2F%2Felsewhere"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, _ = do(req)
	if cs := resp.Cookies(); len(cs) != 1 || cs[0].Value != url.QueryEscape("status=all") {
		t.Errorf("cookie kept a foreign key: %v", cs)
	}
}
