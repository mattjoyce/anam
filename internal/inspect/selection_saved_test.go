package inspect_test

import (
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Saved selections (card 206) on the inspector's pages, over a real ledger and a real
// operational store.

func savedServer(t *testing.T, maxReadBytes int) *httptest.Server {
	t.Helper()
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	a, err := ops.Open(t.Context(), filepath.Join(t.TempDir(), "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	return selectServerWith(t, func(d *inspect.Deps) {
		d.Selections = selection.NewStore(a)
		d.MaxReadBytes = maxReadBytes
	})
}

var savedCodeRE = regexp.MustCompile(`id="saved-code" type="text" readonly value="sel:([0-9a-f]+)"`)

// save posts a program to the select page's save and answers the code the page it
// lands on shows.
func save(t *testing.T, srv *httptest.Server, program string) (string, string) {
	t.Helper()
	resp, err := http.PostForm(srv.URL+"/select/save", url.Values{"program": {program}}) //nolint:gosec // test URL from httptest
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, body, landed := fetch(t, resp.Request.URL.String())
	m := savedCodeRE.FindStringSubmatch(body)
	if resp.StatusCode != http.StatusOK || m == nil {
		t.Fatalf("save = %d at %s, no code shown:\n%s", resp.StatusCode, landed, body)
	}
	if landed.Query().Get("sel") != m[1] {
		t.Fatalf("landed at %s, want the page at the code %s", landed, m[1])
	}
	return m[1], body
}

func TestSelectSave_ShowsTheCodeAndOpensWithTheChips(t *testing.T) {
	t.Parallel()
	srv := savedServer(t, 512<<10)
	_, page, _ := fetch(t, srv.URL+"/select?type=health.weight&source=withings")
	prog := program(t, page)
	code, body := save(t, srv, prog)
	if len(code) != selection.CodeLen {
		t.Fatalf("code %q, want %d hex", code, selection.CodeLen)
	}
	// Opened at its code, the chips are drawn from the program, and the boxes too.
	if chips := removeRE.FindAllStringSubmatch(body, -1); len(chips) != 2 {
		t.Fatalf("the page at the code drew %d chips, want type and source", len(chips))
	}
	if all, _ := boxes(body); len(all) != 3 {
		t.Errorf("the page at the code lists %d facets, want the three weigh-ins", len(all))
	}
	for _, want := range []string{`href="/?sel=` + code + `"`, `href="/link?sel=` + code + `"`, `href="/?sel=` + code + `&amp;selmode=highlight"`} {
		if !strings.Contains(body, want) {
			t.Errorf("the page at the code lacks %s", want)
		}
	}
	// The same program saved again is the same code.
	if again, _ := save(t, srv, "\n"+prog+"\n"); again != code {
		t.Errorf("one program, two codes: %s and %s", code, again)
	}
	// A different program is another.
	if other, _ := save(t, srv, program(t, mustFetch(t, srv.URL+"/select?type=health.blood-pressure"))); other == code {
		t.Errorf("two programs share the code %s", code)
	}
}

func mustFetch(t *testing.T, u string) string {
	t.Helper()
	status, body, _ := fetch(t, u)
	if status != http.StatusOK {
		t.Fatalf("GET %s = %d:\n%s", u, status, body)
	}
	return body
}

var (
	pointRE   = regexp.MustCompile(`<a class="pt([^"]*)" href="/facet\?id=([^"]+)"`)
	selHeadRE = regexp.MustCompile(`<strong id="sel-header">selection sel:([0-9a-f]+), (\d+) facets?</strong>`)
)

// marks is the facet ids the band draws as marks, and those drawn picked.
func marks(body string) (drawn, picked map[string]bool) {
	drawn, picked = map[string]bool{}, map[string]bool{}
	for _, m := range pointRE.FindAllStringSubmatch(body, -1) {
		drawn[m[2]] = true
		if strings.Contains(m[1], "picked") {
			picked[m[2]] = true
		}
	}
	return drawn, picked
}

func TestTimelineSelection_OnlyAndHighlight(t *testing.T) {
	t.Parallel()
	srv := savedServer(t, 512<<10)
	code, _ := save(t, srv, program(t, mustFetch(t, srv.URL+"/select?type=health.weight")))
	weights := ids("w1", "w2", "old")

	only := mustFetch(t, srv.URL+"/?sel="+code)
	if m := selHeadRE.FindStringSubmatch(only); m == nil || m[1] != code || m[2] != "3" {
		t.Fatalf("header %v, want selection sel:%s, 3 facets", m, code)
	}
	drawn, _ := marks(only)
	if len(drawn) != 3 {
		t.Errorf("drawn alone, the band holds %d marks, want the 3 weigh-ins: %v", len(drawn), drawn)
	}
	for _, id := range weights {
		if !drawn[id] {
			t.Errorf("drawn alone, %s is missing", id)
		}
	}
	// Every link the page makes keeps the selection.
	if !strings.Contains(only, `<input type="hidden" name="sel" value="`+code+`">`) {
		t.Error("the controls form drops the selection")
	}

	high := mustFetch(t, srv.URL+"/?sel="+code+"&selmode=highlight")
	if m := selHeadRE.FindStringSubmatch(high); m == nil || m[2] != "3" {
		t.Fatalf("highlighted header %v, want 3 facets", m)
	}
	drawn, picked := marks(high)
	if len(drawn) != len(picks) || len(picked) != 3 {
		t.Errorf("highlighted, the band draws %d marks with %d picked, want %d with 3", len(drawn), len(picked), len(picks))
	}
	for _, id := range weights {
		if !picked[id] {
			t.Errorf("highlighted, %s is not picked out", id)
		}
	}
	if !strings.Contains(high, `class="pt faded"`) {
		t.Error("highlighted, the rest are not faded")
	}

	// Within a window, the facets the program selects in it.
	windowed := mustFetch(t, srv.URL+"/?sel="+code+"&from=2026-10-01&to=2026-10-31")
	if m := selHeadRE.FindStringSubmatch(windowed); m == nil || m[2] != "2" {
		t.Errorf("in October, header %v, want the 2 readings of the weigh-in", m)
	}
}

func TestTimelineSelection_UnknownCodeIs404(t *testing.T) {
	t.Parallel()
	srv := savedServer(t, 512<<10)
	for _, u := range []string{"/?sel=00000000", "/select?sel=sel:00000000"} {
		status, body, _ := fetch(t, srv.URL+u)
		if status != http.StatusNotFound || !strings.Contains(body, "No saved selection opens as <span class=\"mono\">sel:00000000</span>") {
			t.Errorf("GET %s = %d, want a 404 saying so:\n%s", u, status, body)
		}
		if strings.Contains(body, `id="band"`) {
			t.Errorf("GET %s drew a timeline for a code that opens nothing", u)
		}
	}
	if status, _, _ := fetch(t, srv.URL+"/?sel=nothex"); status != http.StatusBadRequest {
		t.Errorf("a string that cannot be a code = %d, want 400", status)
	}
	// Without a store of saved selections, a code is still never an empty timeline.
	plain := selectServer(t)
	if status, _, _ := fetch(t, plain.URL+"/?sel=00000000"); status != http.StatusNotFound {
		t.Errorf("an inspector with no saved selections answers a code %d, want 404", status)
	}
}

func TestTimelineSelection_OverTheBoundAsksForANarrowerWindow(t *testing.T) {
	t.Parallel()
	big := savedServer(t, 512<<10)
	prog := program(t, mustFetch(t, big.URL+"/select?type=health.weight"))
	srv := savedServer(t, 64)
	code, _ := saveDirect(t, srv, prog)
	body := mustFetch(t, srv.URL+"/?sel="+code)
	if !strings.Contains(body, `id="sel-too-large"`) || !strings.Contains(html.UnescapeString(body), "Narrow the window") {
		t.Fatalf("over the bound, the timeline does not say so:\n%s", excerpt(body, "sel-drawn"))
	}
	if drawn, _ := marks(body); len(drawn) != 0 {
		t.Errorf("over the bound, the band still draws %d marks as if selected", len(drawn))
	}
}

// saveDirect saves through the page's form where the page itself cannot be drawn
// within the bound: the save writes, and lands on a page that may refuse to list.
func saveDirect(t *testing.T, srv *httptest.Server, program string) (string, int) {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.PostForm(srv.URL+"/select/save", url.Values{"program": {program}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	loc, err := url.Parse(resp.Header.Get("Location"))
	if err != nil || resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save = %d to %q", resp.StatusCode, resp.Header.Get("Location"))
	}
	return loc.Query().Get("sel"), resp.StatusCode
}
