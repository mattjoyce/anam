package daemon

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// Card 199, slice 3: the entry points into the selection page and the whole walk from a
// facet's page to a saved link, through a real daemon. The facets are synthetic Garmin
// heart-rate alerts and a session on one day; nothing here is a real reading.

var linkThisRE = regexp.MustCompile(`<a href="(/select\?tick=[^"]+)">link this…</a>`)

// alertEnv is a heart-rate alert at a UTC minute on 21 October 2026.
func alertEnv(t *testing.T, version int, minute string) string {
	t.Helper()
	at := "2026-10-21T" + minute + ":00Z"
	return fmt.Sprintf(`{"type": "health.heart-rate-alert", "definition_version": %d, "payload": {
		"source": "garmin", "kind": "hr_high_alert", "fact_time": %q, "heart_rate": 124, "threshold": 120, "unit": "bpm",
		"source_time": {"value": %q, "zone": "UTC", "basis": "source"}}}`, version, at, at)
}

// sessionEnv is a walking session on 21 October 2026.
func sessionEnv(version int) string {
	return fmt.Sprintf(`{"type": "activity.exercise-session.garmin", "definition_version": %d, "payload": {
		"id": "garmin:session:2026-10-21T04:00:00Z", "activity_name": "walking",
		"effective_time_frame": {"time_interval": {"start_date_time": "2026-10-21T15:00:00+11:00", "duration": {"value": 1800, "unit": "sec"}}}}}`, version)
}

// servingDay starts a daemon whose Workbench enters links and whose test importer may
// write heart-rate alerts and Garmin sessions, and admits five alerts and a session. It
// returns the alerts' facet ids in minute order and the session's.
func servingDay(t *testing.T, workbench bool) (d *Daemon, base string, alerts []string, session string) {
	t.Helper()
	cfg := importerConfig(t)
	raw, err := os.ReadFile(cfg.ImportersFile)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(raw), "types: [health.weight, test.reading]",
		"types: [health.weight, test.reading, health.heart-rate-alert, activity.exercise-session.garmin]", 1)
	if workbench {
		body += "  - source: workbench\n    types: [record.edge]\n"
	}
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d, base, _ = serveImporters(t, cfg)
	alertDef, _ := d.defs.current.Load().Lookup("health.heart-rate-alert")
	sessionDef, _ := d.defs.current.Load().Lookup("activity.exercise-session.garmin")
	envs := []string{
		alertEnv(t, alertDef.Version, "03:10"), alertEnv(t, alertDef.Version, "03:20"), alertEnv(t, alertDef.Version, "03:30"),
		alertEnv(t, alertDef.Version, "03:40"), alertEnv(t, alertDef.Version, "03:50"), sessionEnv(sessionDef.Version),
	}
	var adm api.Admission
	call(t, "POST", base+"/v1/envelopes", importerToken, batch(envs...), http.StatusOK, &adm)
	if len(adm.Results) != len(envs) {
		t.Fatalf("admitted %d of %d: %+v", len(adm.Results), len(envs), adm.Results)
	}
	for i, r := range adm.Results {
		if r.FacetID == "" {
			t.Fatalf("envelope %d was not admitted: %+v", i, r)
		}
		if i < 5 {
			alerts = append(alerts, r.FacetID)
		} else {
			session = r.FacetID
		}
	}
	return d, base, alerts, session
}

// The card's acceptance, through the real pages. From an alert's page, "link this…"
// opens the selection page with it ticked; the date and the concept are added (in the
// order the person likes), the ticks settled, a comment written and saved. One
// record.edge with four facets in to, and each one's page lists it.
func TestLinkThisFromAnAlertToASavedLinkOfFourFacets(t *testing.T) {
	d, base, alerts, session := servingDay(t, true)
	first, decoy := alerts[0], alerts[4]

	// 1. The alert's page offers "link this…" at the selection page with it ticked.
	_, alertPage := page(t, "GET", base+"/facet?id="+first, nil, nil)
	m := linkThisRE.FindStringSubmatch(alertPage)
	if m == nil {
		t.Fatalf("the alert's page offers no link this:\n%s", alertPage)
	}
	start, err := url.Parse(html.UnescapeString(m[1]))
	if err != nil || start.Path != "/select" || start.Query().Get("tick") != first {
		t.Fatalf("link this opens %q, want /select with %s ticked", m[1], first)
	}
	_, opened := page(t, "GET", base+start.RequestURI(), nil, nil)
	if !strings.Contains(opened, `<li class="tick">`) || !strings.Contains(opened, `href="/facet?id=`+first+`"`) {
		t.Fatalf("the picker opened without the alert in its basket:\n%s", opened)
	}
	if strings.Contains(opened, `name="on"`) {
		t.Error("the picker lists matches before any criterion is added")
	}

	// 2. Add the date, then the concepts: the matches are the five alerts and the
	// session, all of them ticked, and the first is the one in the basket.
	q := start.Query()
	q.Set("from", "2026-10-21")
	q.Set("to", "2026-10-21")
	_, dated := page(t, "GET", base+"/select?"+q.Encode(), nil, nil)
	if !strings.Contains(dated, `<strong id="matched">6</strong>`) {
		t.Fatalf("the day should match 6 facets:\n%s", dated)
	}
	q.Add("type", "health.heart-rate-alert")
	q.Add("type", "activity.exercise-session.")
	_, listed := page(t, "GET", base+"/select?"+q.Encode(), nil, nil)
	if !strings.Contains(listed, `<strong id="matched">6</strong>`) {
		t.Fatalf("the concepts should match 6 facets:\n%s", listed)
	}

	// 3. Untick the last alert and keep the rest in the basket: the form sends every
	// box it shows and the ones ticked.
	shown := append(append([]string(nil), alerts...), session)
	on := []string{alerts[0], alerts[1], alerts[2], session}
	post := q
	post["shown"], post["on"] = shown, on
	post.Set("act", "keep")
	_, kept := page(t, "GET", base+"/select?"+post.Encode(), nil, nil)
	h := handoffLink.FindStringSubmatch(kept)
	if h == nil {
		t.Fatalf("no handoff after keeping the ticked:\n%s", kept)
	}

	// 4. Use the selection: the form holds the four, and not the one unticked.
	_, form := page(t, "GET", base+html.UnescapeString(h[1]), nil, nil)
	if !strings.Contains(form, "4 in the link") || strings.Contains(form, decoy) {
		t.Fatalf("the form should hold the four and not %s:\n%s", decoy, form)
	}
	want := append([]string(nil), on...)
	for _, id := range want {
		if !strings.Contains(form, `name="keep" value="`+id+`" checked`) {
			t.Errorf("the form lacks %s", id)
		}
	}

	// 5. Write the comment and save.
	status, body := page(t, "POST", base+"/link", linkForm(edgeVersion(t, d), "about", "A run of high readings, then a walk.", want, nil, nil), nil)
	_, edge := wantEdgeID("about", want, nil)
	if status != http.StatusOK || !strings.Contains(body, "admitted") || !strings.Contains(body, edge) {
		t.Fatalf("save = %d, want %s admitted:\n%s", status, edge, body)
	}
	saved, err := d.read.CurrentVersion(t.Context(), edge)
	if err != nil {
		t.Fatal(err)
	}
	sorted := append([]string(nil), want...)
	sort.Strings(sorted)
	if want := `"to":["` + strings.Join(sorted, `","`) + `"]`; !strings.Contains(string(saved.Payload.Bytes()), want) {
		t.Errorf("payload %s lacks %s", saved.Payload.Bytes(), want)
	}
	for _, id := range want {
		if !pageLists(t, base, id, edge) {
			t.Errorf("the page of %s does not list the link", id)
		}
	}
	if pageLists(t, base, decoy, edge) {
		t.Errorf("the unticked alert's page lists the link")
	}
}

// The same result is reached by adding the concept first and the date second (the
// card's second acceptance): the address of the picker is the same either way.
func TestTheOrderOfCriteriaReachesTheSameSelection(t *testing.T) {
	_, base, alerts, session := servingDay(t, true)
	tick := url.Values{"tick": {alerts[0]}}
	dateFirst := url.Values{"tick": {alerts[0]}, "from": {"2026-10-21"}, "to": {"2026-10-21"}}
	dateFirst.Add("type", "health.heart-rate-alert")
	conceptFirst := url.Values{"tick": tick["tick"], "type": {"health.heart-rate-alert"}}
	conceptFirst.Set("from", "2026-10-21")
	conceptFirst.Set("to", "2026-10-21")
	_, a := page(t, "GET", base+"/select?"+dateFirst.Encode(), nil, nil)
	_, b := page(t, "GET", base+"/select?"+conceptFirst.Encode(), nil, nil)
	ha, hb := handoffLink.FindStringSubmatch(a), handoffLink.FindStringSubmatch(b)
	if ha == nil || hb == nil || ha[1] != hb[1] {
		t.Fatalf("the two orders hand off differently:\n%v\n%v", ha, hb)
	}
	if !strings.Contains(a, `<strong id="matched">5</strong>`) || strings.Contains(a, session) {
		t.Errorf("the alerts alone should match 5, not the session:\n%s", a)
	}
}

// The timeline's own selection (ctrl-click or shift-drag marks) opens the picker with
// the marked facets in the basket. The page carries a "link these…" anchor the
// selection script points at /select with one tick per mark; without a Workbench that
// enters links, there is none.
func TestTimelineOffersLinkTheseOnlyWhereLinksCanBeEntered(t *testing.T) {
	_, withBench, _, _ := servingDay(t, true)
	_, plain, _, _ := servingDay(t, false)

	_, body := page(t, "GET", withBench+"/", nil, nil)
	for _, want := range []string{`<a class="button" id="sel-link" href="/select">link these…</a>`, `'/select?' + [...selected.values()].map(m => 'tick=' + encodeURIComponent(m.id))`} {
		if !strings.Contains(body, want) {
			t.Errorf("the timeline lacks %q", want)
		}
	}
	_, body = page(t, "GET", plain+"/", nil, nil)
	if strings.Contains(body, `id="sel-link"`) {
		t.Error("the timeline offers link these without a Workbench that enters links")
	}
}
