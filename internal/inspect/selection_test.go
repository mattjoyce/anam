package inspect_test

import (
	"context"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The selection page (card 199) over a real ledger: a weigh-in of two readings and a
// blood pressure at one moment, two heart-rate alerts, a run, an older weight.

type pick struct{ ref, typ, source string }

var picks = []struct {
	pick
	at time.Time
}{
	{pick{"w1", "health.weight", "withings"}, time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC)},
	{pick{"w2", "health.weight", "withings"}, time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC)},
	{pick{"bp1", "health.blood-pressure", "withings"}, time.Date(2026, 10, 2, 7, 0, 0, 0, time.UTC)},
	{pick{"hr1", "health.heart-rate.alert", "garmin"}, time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)},
	{pick{"hr2", "health.heart-rate.alert", "garmin"}, time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)},
	{pick{"run1", "activity.exercise-session.run", "garmin"}, time.Date(2026, 10, 3, 11, 0, 0, 0, time.UTC)},
	{pick{"old", "health.weight", "withings"}, time.Date(2026, 9, 1, 7, 0, 0, 0, time.UTC)},
}

func pid(ref string) string { return facet.NewID("sel:" + ref) }

func selectServer(t *testing.T) *httptest.Server {
	t.Helper()
	return selectServerWith(t, func(*inspect.Deps) {})
}

// selectServerWith is selectServer with its Deps changed by edit.
func selectServerWith(t *testing.T, edit func(*inspect.Deps)) *httptest.Server {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	for i, p := range picks {
		f := seedFacetNoT("sel:"+p.ref, p.typ, `{"n": `+strconv.Itoa(i)+`}`)
		f.FactTime, f.Provenance.Source = p.at, p.source
		ev := event(facet.EventAdmitted, f, int64(i+1))
		if err := l.Admit(ctx, ledger.Watermark(i), f, ev); err != nil {
			t.Fatalf("Admit %s: %v", p.ref, err)
		}
	}
	deps := inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, MaxReadBytes: 512 << 10}
	edit(&deps)
	srv := httptest.NewServer(inspect.Handler(deps))
	t.Cleanup(srv.Close)
	return srv
}

// fetch GETs a page, following redirects, and answers the address it landed on.
func fetch(t *testing.T, u string) (int, string, *url.URL) {
	t.Helper()
	resp, err := http.Get(u) //nolint:gosec // test URL from httptest
	if err != nil {
		t.Fatalf("GET %s: %v", u, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return resp.StatusCode, string(b), resp.Request.URL
}

var (
	boxRE     = regexp.MustCompile(`class="member" type="checkbox" name="on" value="([^"]+)"( checked)?`)
	shownRE   = regexp.MustCompile(`name="shown" value="([^"]+)"`)
	matchedRE = regexp.MustCompile(`<strong id="matched">(\d+)</strong>`)
	pickedRE  = regexp.MustCompile(`<strong id="picked">(\d+)</strong>`)
	removeRE  = regexp.MustCompile(`<li class="chip">([^<]+)<span class="count">(\d+)</span><a class="remove" href="([^"]+)"`)
	hiddenRE  = regexp.MustCompile(`<input type="hidden" name="([^"]+)" value="([^"]*)">`)
	programRE = regexp.MustCompile(`(?s)<textarea class="program" name="program" spellcheck="false">(.*?)</textarea>`)
)

func number(t *testing.T, re *regexp.Regexp, body string) int {
	t.Helper()
	m := re.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no %s in the page:\n%s", re, body)
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// boxes is the facet ids drawn with a checkbox, and which are ticked.
func boxes(body string) (all, ticked []string) {
	for _, m := range boxRE.FindAllStringSubmatch(body, -1) {
		all = append(all, m[1])
		if m[2] != "" {
			ticked = append(ticked, m[1])
		}
	}
	sort.Strings(all)
	sort.Strings(ticked)
	return all, ticked
}

func ids(refs ...string) []string {
	var out []string
	for _, r := range refs {
		out = append(out, pid(r))
	}
	sort.Strings(out)
	return out
}

func program(t *testing.T, body string) string {
	t.Helper()
	m := programRE.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no program in the page")
	}
	return html.UnescapeString(m[1])
}

func TestSelect_CountIsItsRowsAndAWeighInIsOneRow(t *testing.T) {
	t.Parallel()
	srv := selectServer(t)
	code, body, _ := fetch(t, srv.URL+"/select?type=health.weight&from=2026-10-01")
	if code != 200 {
		t.Fatalf("GET = %d\n%s", code, body)
	}
	all, ticked := boxes(body)
	if n := number(t, matchedRE, body); n != len(all) || n != 2 {
		t.Errorf("count %d, boxes %d, want 2 of each", n, len(all))
	}
	if want := ids("w1", "w2"); strings.Join(all, ",") != strings.Join(want, ",") || len(ticked) != 2 {
		t.Errorf("boxes %v ticked %v, want %v all ticked", all, ticked, want)
	}
	if !strings.Contains(body, "in 1 row.") {
		t.Error("the two readings of one weigh-in are not one row")
	}
	if number(t, pickedRE, body) != 2 {
		t.Error("the program does not pick what the criteria match")
	}
}

func TestSelect_NoCriteriaListsNothingAndPicksTheBasket(t *testing.T) {
	t.Parallel()
	srv := selectServer(t)
	_, body, _ := fetch(t, srv.URL+"/select?tick="+pid("hr1"))
	if all, _ := boxes(body); len(all) != 0 || !strings.Contains(body, "Add a criterion") {
		t.Errorf("no criteria drew %d boxes", len(all))
	}
	if number(t, pickedRE, body) != 1 {
		t.Error("the basket alone is not the selection")
	}
}

// A tick and its side outlive the criterion that found it.
func TestSelect_TheBasketSurvivesACriteriaChange(t *testing.T) {
	t.Parallel()
	srv := selectServer(t)
	hr1 := pid("hr1")
	_, body, _ := fetch(t, srv.URL+"/select?type=health.heart-rate.&tick="+hr1+"&fromside="+hr1)
	chips := removeRE.FindAllStringSubmatch(body, -1)
	if len(chips) != 1 {
		t.Fatalf("chips = %v, want the one type", chips)
	}
	_, after, landed := fetch(t, srv.URL+html.UnescapeString(chips[0][3]))
	if landed.Query().Get("type") != "" {
		t.Fatalf("the chip did not go: %s", landed)
	}
	if landed.Query().Get("tick") != hr1 || landed.Query().Get("fromside") != hr1 {
		t.Errorf("the basket did not survive removing the criterion: %s", landed)
	}
	if !strings.Contains(after, `<span class="side">from</span>`) {
		t.Error("the tick lost its side")
	}
	if !strings.Contains(program(t, after), `pick("`+hr1+`").`) {
		t.Errorf("the program does not hold the tick:\n%s", program(t, after))
	}

	// Adding a criterion through the form keeps the basket too: it rides as hidden inputs.
	q := formState(body)
	q.Add("type", "activity.exercise-session.")
	_, added, landed := fetch(t, srv.URL+"/select?"+q.Encode())
	if landed.Query().Get("tick") != hr1 || !strings.Contains(added, `<span class="side">from</span>`) {
		t.Errorf("adding a criterion lost the basket: %s", landed)
	}
	if all, _ := boxes(added); len(all) != 3 {
		t.Errorf("two types listed %d facets, want the two alerts and the run", len(all))
	}
}

// formState is the hidden inputs of the criteria form and its visible values, as a
// browser submits them with the add inputs left blank.
func formState(body string) url.Values {
	form := body[strings.Index(body, `<form class="controls"`):]
	form = form[:strings.Index(form, "</form>")]
	q := url.Values{}
	for _, m := range hiddenRE.FindAllStringSubmatch(form, -1) {
		q.Add(m[1], html.UnescapeString(m[2]))
	}
	for _, name := range []string{"from", "to"} {
		if m := regexp.MustCompile(`type="date" name="` + name + `" id="` + name + `" value="([^"]*)"`).FindStringSubmatch(form); m != nil && m[1] != "" {
			q.Set(name, html.UnescapeString(m[1]))
		}
	}
	return q
}

// Adding the date then the type selects what adding the type then the date does.
func TestSelect_TheOrderOfCriteriaDoesNotMatter(t *testing.T) {
	t.Parallel()
	srv := selectServer(t)
	step := func(body string, add url.Values) string {
		q := formState(body)
		for k, v := range add {
			q[k] = append(q[k], v...)
		}
		_, next, _ := fetch(t, srv.URL+"/select?"+q.Encode())
		return next
	}
	date := url.Values{"from": {"2026-10-02"}, "to": {"2026-10-03"}}
	typ := url.Values{"type": {"health.weight"}}
	_, start, _ := fetch(t, srv.URL+"/select")

	dateFirst := step(step(start, date), typ)
	typeFirst := step(step(start, typ), date)
	a, _ := boxes(dateFirst)
	b, _ := boxes(typeFirst)
	if strings.Join(a, ",") != strings.Join(b, ",") || strings.Join(a, ",") != strings.Join(ids("w1", "w2"), ",") {
		t.Errorf("date then type lists %v, type then date %v", a, b)
	}
	if program(t, dateFirst) != program(t, typeFirst) {
		t.Errorf("two orders, two programs:\n%s\n%s", program(t, dateFirst), program(t, typeFirst))
	}
}

// Unticking a match excludes it; keeping puts the ticked in the basket.
func TestSelect_TicksAndKeep(t *testing.T) {
	t.Parallel()
	srv := selectServer(t)
	w1, w2 := pid("w1"), pid("w2")
	q := url.Values{"type": {"health.weight"}, "from": {"2026-10-01"}, "shown": {w1, w2}, "on": {w1}, "act": {"update"}}
	_, body, landed := fetch(t, srv.URL+"/select?"+q.Encode())
	if landed.Query().Get("untick") != w2 || landed.Query().Has("shown") {
		t.Fatalf("unticking w2 landed on %s", landed)
	}
	if _, ticked := boxes(body); strings.Join(ticked, ",") != w1 || number(t, pickedRE, body) != 1 {
		t.Errorf("ticked %v, picked %d, want w1 alone", ticked, number(t, pickedRE, body))
	}
	if !strings.Contains(program(t, body), "not unpick(E)") {
		t.Error("the exclusion is not in the program")
	}

	q.Set("act", "keep")
	_, _, landed = fetch(t, srv.URL+"/select?"+q.Encode())
	if landed.Query().Get("tick") != w1 || landed.Query().Get("untick") != w2 {
		t.Errorf("keeping landed on %s, want w1 ticked and w2 unticked", landed)
	}
}

// A program the criteria could make draws them; any other runs as text, chips off.
func TestSelect_AnEditedProgram(t *testing.T) {
	t.Parallel()
	srv := selectServer(t)
	inside := `pick(E) :- entry(E, T, _, _, _, S, _), T = "health.weight", S = "withings".`
	_, body, landed := fetch(t, srv.URL+"/select?program="+url.QueryEscape(inside))
	if landed.Query().Get("type") != "health.weight" || landed.Query().Get("source") != "withings" || landed.Query().Has("program") {
		t.Errorf("a program in the grammar landed on %s", landed)
	}
	if len(removeRE.FindAllString(body, -1)) != 2 {
		t.Error("the criteria are not drawn from the program")
	}

	outside := `pick(E) :- entry(E, "health.weight", _, _, _, _, _), entry(F, "health.blood-pressure", At, _, _, _, _), entry(E, _, At, _, _, _, _).`
	code, body, landed := fetch(t, srv.URL+"/select?program="+url.QueryEscape(outside))
	if code != 200 || !landed.Query().Has("program") {
		t.Fatalf("a program outside the grammar = %d at %s", code, landed)
	}
	if !strings.Contains(body, "outside what the criteria make") || strings.Contains(body, `<form class="controls"`) {
		t.Error("the chips are not off, or the page does not say so")
	}
	if all, _ := boxes(body); len(all) != 0 {
		t.Errorf("a program run as text drew %d boxes", len(all))
	}
	listed := shownRE.FindAllStringSubmatch(body, -1)
	var got []string
	for _, m := range listed {
		got = append(got, m[1])
	}
	sort.Strings(got)
	if want := ids("w1", "w2"); strings.Join(got, ",") != strings.Join(want, ",") || number(t, matchedRE, body) != 2 {
		t.Errorf("the join picked %v, want the weigh-in's weights %v", got, want)
	}

	_, bad, _ := fetch(t, srv.URL+"/select?program="+url.QueryEscape("pick(E) :- nothing(E)."))
	if !strings.Contains(bad, "unknown relation nothing/1") {
		t.Error("a program the evaluator refuses does not say why")
	}
}

func TestSelect_OverTheLimitListsNothing(t *testing.T) {
	t.Parallel()
	srv := seedBig(t, 520)
	_, body, _ := fetch(t, srv.URL+"/select?type=activity.steps")
	if all, _ := boxes(body); len(all) != 0 || !strings.Contains(body, `id="too-many"`) {
		t.Errorf("520 rows drew %d boxes and the page does not ask for another criterion", len(all))
	}
	if number(t, matchedRE, body) != 520 {
		t.Error("the count is not stated when the rows are not drawn")
	}
	_, day, _ := fetch(t, srv.URL+"/select?type=activity.steps&from=2026-03-02&to=2026-03-02")
	if all, _ := boxes(day); len(all) != 50 || strings.Contains(day, `id="too-many"`) {
		t.Errorf("one day drew %d boxes, want 50", len(all))
	}
}

func TestSelect_HandoffAndRefusals(t *testing.T) {
	t.Parallel()
	srv := selectServer(t)
	hr1 := pid("hr1")
	_, body, _ := fetch(t, srv.URL+"/select?p=bp&type=health.blood-pressure&tick="+hr1+"&fromside="+hr1)
	m := regexp.MustCompile(`<a id="handoff" href="([^"]+)"`).FindStringSubmatch(body)
	if m == nil {
		t.Fatal("no handoff link")
	}
	h, err := url.Parse(html.UnescapeString(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	if h.Path != "/link" || h.Query().Get("p") != "bp" || h.Query().Get("tick") != hr1 || h.Query().Get("fromside") != hr1 ||
		!strings.Contains(h.Query().Get("program"), `T = "health.blood-pressure"`) {
		t.Errorf("handoff = %s", h)
	}
	if !strings.Contains(body, "not served by this projection") {
		t.Error("a tick the projection does not serve is not marked")
	}

	for u, want := range map[string]int{
		"/select?p=nope":                  404,
		"/select?from=2026-13-01":         400,
		"/select?type=health.":            400,
		"/select?standing=sometimes":      400,
		"/select?to=2026-10-01T00:00:00Z": 400,
	} {
		if code, _, _ := fetch(t, srv.URL+u); code != want {
			t.Errorf("GET %s = %d, want %d", u, code, want)
		}
	}
}
