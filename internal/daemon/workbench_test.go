package daemon

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// The Workbench pages (card 188, S4) are tested through a real daemon: a real ledger
// and operational store under t.TempDir(), the inspector mounted at "/", and the
// workbench principal declared in the importers file as an operator would declare it.

// workbenchServing is a running daemon whose importers file lists the workbench with
// the given types, and its URL.
func workbenchServing(t *testing.T, types string) (*Daemon, string) {
	t.Helper()
	cfg := importerConfig(t)
	f, err := os.OpenFile(cfg.ImportersFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString("  - source: workbench\n    types: [" + types + "]\n"); err != nil {
		t.Fatal(err)
	}
	d, base, _ := serveImporters(t, cfg)
	return d, base
}

// page sends a GET or a form POST to the inspector as the operator and returns the
// status and body.
func page(t *testing.T, method, target string, form url.Values, headers map[string]string) (int, string) {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

// inputTag is the form control named name in a page, whole, so a test can say what
// kind it is and whether it is locked.
func inputTag(t *testing.T, body, name string) string {
	t.Helper()
	m := regexp.MustCompile(`<(input|select)[^>]*name="f\.` + regexp.QuoteMeta(name) + `"[^>]*>`).FindString(body)
	if m == "" {
		t.Fatalf("the form has no control for %q:\n%s", name, body)
	}
	return m
}

// doseForm is what the dose form posts for a melatonin dose.
func doseForm(version int, taken, amount string) url.Values {
	return url.Values{
		"type": {"health.medication.dose"}, "v": {strconv.Itoa(version)},
		"f.drug": {"melatonin"}, "f.amount": {amount}, "f.unit": {"mg"},
		"f.taken_at": {taken}, "f.reason": {"to help sleep"}, "zone": {"Australia/Sydney"}, "offset": {"+11:00"},
	}
}

// The Definitions page offers an Enter link to the types in the workbench's list and
// to no other; the daemon tells the pages the list, and the pages do not read the file.
func TestDefinitionsPageMarksTheEnterableTypes(t *testing.T) {
	_, base := workbenchServing(t, "health.medication.dose")
	status, body := page(t, "GET", base+"/definitions", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /definitions = %d", status)
	}
	if n := strings.Count(body, `href="/entry?type=`); n != 1 {
		t.Fatalf("the page links %d forms, want exactly 1", n)
	}
	if !strings.Contains(body, `href="/entry?type=health.medication.dose"`) {
		t.Fatalf("the dose type has no Enter link:\n%s", body)
	}
	row := regexp.MustCompile(`(?s)<tr>\s*<td><a class="mono" href="/definition\?type=health.weight">.*?</tr>`).FindString(body)
	if !strings.Contains(row, "not enterable") || strings.Contains(row, "/entry?") {
		t.Fatalf("health.weight's row = %s, want no link and a reason", row)
	}

	// A direct request for a form off the list is refused.
	if status, _ := page(t, "GET", base+"/entry?type=health.weight", nil, nil); status != http.StatusForbidden {
		t.Fatalf("GET /entry for a type off the list = %d, want 403", status)
	}
	if status, _ := page(t, "POST", base+"/entry", url.Values{"type": {"health.weight"}, "v": {"1"}}, nil); status != http.StatusForbidden {
		t.Fatalf("POST /entry for a type off the list = %d, want 403", status)
	}
}

// Without a workbench principal the pages offer nothing: no link, no route.
func TestNoWorkbenchPrincipalMeansNoEntryPages(t *testing.T) {
	_, base, _ := serveImporters(t, importerConfig(t))
	_, body := page(t, "GET", base+"/definitions", nil, nil)
	if strings.Contains(body, `href="/entry?`) {
		t.Fatalf("a daemon with no workbench links a form")
	}
	if status, _ := page(t, "GET", base+"/entry?type=health.medication.dose", nil, nil); status != http.StatusNotFound {
		t.Fatalf("GET /entry with no workbench = %d, want 404", status)
	}
}

// The dose form is drawn from the Definition: one control per property in the kind its
// schema names, the guidance word for word, the version it was drawn from, and no id.
func TestDoseFormIsDrawnFromTheDefinition(t *testing.T) {
	d, base := workbenchServing(t, "health.medication.dose")
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	status, body := page(t, "GET", base+"/entry?type=health.medication.dose", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET /entry = %d: %s", status, body)
	}
	for name, want := range map[string][]string{
		"taken_at":    {`type="datetime-local"`, "required"},
		"drug":        {`type="text"`, "required", `placeholder="melatonin"`},
		"amount":      {`type="number"`, `step="any"`, "required", `placeholder="5"`},
		"unit":        {`type="text"`, "required", `placeholder="mg"`},
		"course":      {`type="number"`, `step="any"`},
		"reason":      {`type="text"`, `placeholder="for sleep"`},
		"source_note": {`type="text"`},
	} {
		tag := inputTag(t, body, name)
		for _, w := range want {
			if !strings.Contains(tag, w) {
				t.Errorf("control %s = %s, want %s", name, tag, w)
			}
		}
		if strings.Contains(tag, "readonly") {
			t.Errorf("control %s is locked on a new entry: %s", name, tag)
		}
	}
	if strings.Contains(inputTag(t, body, "course"), "required") {
		t.Errorf("an optional field is marked required")
	}
	if strings.Contains(body, `name="f.id"`) {
		t.Errorf("the form draws the composed id field")
	}
	for _, want := range []string{
		`name="v" value="` + strconv.Itoa(dose.Version) + `"`,         // the version it was drawn from
		"One dose, taken at a moment: a fact that an intake happened", // the guidance, word for word
		"greater than 0", // exclusiveMinimum as a hint
		"or as Australia/Sydney time without one", // the declared zone, when the browser names none
		`<input type="hidden" id="zone" name="zone" value="">`,
		`<input id="offset" name="offset" type="text" value=""`,
		"Intl.DateTimeFormat().resolvedOptions().timeZone", // the page fills the zone from the browser
		dose.IdentityTemplate,                              // the id rule is shown, not rebuilt
		"Anam builds the id from the Definition",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the form lacks %q", want)
		}
	}
}

// The form sends the zone the browser is in (card 198): a London dose at 18:00 with the
// offset the page read lands at 17:00Z, says how its clock was read, and takes the
// offset into its id; a correction carries the zone it was said in.
func TestTheFormSendsTheBrowsersZone(t *testing.T) {
	d, base := workbenchServing(t, "health.medication.dose")
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	form := doseForm(dose.Version, "2026-10-19T18:00", "5")
	form.Set("zone", "Europe/London")
	form.Set("offset", "+01:00")
	status, body := page(t, "POST", base+"/entry", form, nil)
	key := "workbench:med/melatonin/dose/2026-10-19T18:00+01:00"
	wantID := facet.NewID(facet.SourceRef("workbench", key))
	if status != http.StatusOK || !strings.Contains(body, "admitted") || !strings.Contains(body, wantID) {
		t.Fatalf("save = %d, want %s admitted:\n%s", status, wantID, body)
	}
	f, err := d.read.CurrentVersion(context.Background(), wantID)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.FactTime.UTC().Format(time.RFC3339); got != "2026-10-19T17:00:00Z" {
		t.Errorf("fact time = %s, want 2026-10-19T17:00:00Z, 18:00 in London", got)
	}
	for _, want := range []string{`"id":"` + key + `"`, `"taken_at":"2026-10-19 18:00"`,
		`"source_time":{"basis":"source","offset":"+01:00","value":"2026-10-19 18:00","zone":"Europe/London"}`} {
		if !strings.Contains(string(f.Payload.Bytes()), want) {
			t.Errorf("payload %s lacks %s", f.Payload.Bytes(), want)
		}
	}

	_, edit := page(t, "GET", base+"/entry?type=health.medication.dose&edit="+wantID, nil, nil)
	for _, want := range []string{`name="zone" value="Europe/London"`, `name="offset" type="text" value="&#43;01:00"`, "the zone this fact was said in"} {
		if !strings.Contains(edit, want) {
			t.Errorf("the correction form lacks %s:\n%s", want, edit)
		}
	}
	form.Set("edit", wantID)
	form.Set("f.amount", "10")
	if status, body := page(t, "POST", base+"/entry", form, nil); status != http.StatusOK || !strings.Contains(body, "superseded") {
		t.Fatalf("correction = %d, want superseded:\n%s", status, body)
	}
}

// A schema that needs a nested object or a list is refused with a plain message and no
// form. Blood pressure's body is the bundled type that needs them.
func TestAFormIsRefusedForANestedSchema(t *testing.T) {
	_, base := workbenchServing(t, "health.blood-pressure")
	status, body := page(t, "GET", base+"/entry?type=health.blood-pressure", nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("GET /entry for blood pressure = %d, want 422", status)
	}
	for _, want := range []string{"A form cannot be drawn for this type yet", "effective_time_frame", "Nested objects and lists are not drawn"} {
		if !strings.Contains(body, want) {
			t.Errorf("the refusal lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "<form") {
		t.Errorf("a form was drawn anyway")
	}
}

// A dose entered through the page lands under the workbench with the id the Definition
// composes and the time in the Definition's layout; the same form again is a no-op; a
// changed amount is a new version.
func TestSubmittingTheFormLandsTheDose(t *testing.T) {
	d, base := workbenchServing(t, "health.medication.dose")
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	v := dose.Version
	wantID := facet.NewID(facet.SourceRef("workbench", "workbench:med/melatonin/dose/2026-10-19T22:00+11:00"))

	status, body := page(t, "POST", base+"/entry", doseForm(v, "2026-10-19T22:00", "5"), nil)
	if status != http.StatusOK || !strings.Contains(body, "admitted") || !strings.Contains(body, `href="/facet?id=`+wantID+`"`) {
		t.Fatalf("first save = %d, want admitted with a link to %s:\n%s", status, wantID, body)
	}
	f, err := d.read.CurrentVersion(context.Background(), wantID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Provenance.Source != "workbench" {
		t.Fatalf("source = %q, want workbench", f.Provenance.Source)
	}
	for _, want := range []string{`"taken_at":"2026-10-19 22:00"`, `"amount":5`, `"unit":"mg"`, `"drug":"melatonin"`,
		`"id":"workbench:med/melatonin/dose/2026-10-19T22:00+11:00"`, `"reason":"to help sleep"`} {
		if !strings.Contains(string(f.Payload.Bytes()), want) {
			t.Errorf("payload %s lacks %s", f.Payload.Bytes(), want)
		}
	}
	at := head(t, d)

	status, body = page(t, "POST", base+"/entry", doseForm(v, "2026-10-19T22:00", "5"), nil)
	if status != http.StatusOK || !strings.Contains(body, "no-op") || !strings.Contains(body, "nothing was written") {
		t.Fatalf("second save = %d, want no-op:\n%s", status, body)
	}
	if w := head(t, d); w != at {
		t.Fatalf("W moved from %d to %d on a no-op", at, w)
	}

	status, body = page(t, "POST", base+"/entry", doseForm(v, "2026-10-19T22:00", "10"), nil)
	if status != http.StatusOK || !strings.Contains(body, "superseded") {
		t.Fatalf("changed save = %d, want superseded:\n%s", status, body)
	}
}

// A refusal, a value that is not a number, and a form drawn from an old version each
// write nothing and draw the form again with what was typed, in the gate's words.
func TestAFailedSaveKeepsWhatWasTyped(t *testing.T) {
	d, base := workbenchServing(t, "health.medication.dose")
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	v := dose.Version

	status, body := page(t, "POST", base+"/entry", doseForm(v, "2026-10-19T22:00", "-1"), nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(body, "Not saved.") || !strings.Contains(body, "not compliant") {
		t.Fatalf("a refused amount = %d, want 422 with the gate's words:\n%s", status, body)
	}
	if tag := inputTag(t, body, "amount"); !strings.Contains(tag, `value="-1"`) {
		t.Errorf("amount = %s, want the typed value kept", tag)
	}

	status, body = page(t, "POST", base+"/entry", doseForm(v, "2026-10-19T22:00", "five"), nil)
	if status != http.StatusUnprocessableEntity || !strings.Contains(body, `is not a number`) {
		t.Fatalf("a non-number = %d, want 422 naming the field's problem:\n%s", status, body)
	}

	status, body = page(t, "POST", base+"/entry", doseForm(v+1, "2026-10-19T22:00", "5"), nil)
	if status != http.StatusConflict || !strings.Contains(body, "another definition version") ||
		!strings.Contains(body, "drawn from the Definition now in force") {
		t.Fatalf("a stale form = %d, want 409 and a redrawn form:\n%s", status, body)
	}
	if tag := inputTag(t, body, "drug"); !strings.Contains(tag, `value="melatonin"`) {
		t.Errorf("drug = %s, want the typed value kept", tag)
	}
	if w := head(t, d); w != 0 {
		t.Fatalf("W = %d after failed saves, want 0", w)
	}
}

// Edit opens the same form from a facet the workbench entered, its identity fields
// locked; saving writes a new version of the same facet. Retract tombstones it. A
// facet of another source has neither button, and its retraction is refused.
func TestEditAndRetractAreForTheWorkbenchsOwnFacets(t *testing.T) {
	d, base := workbenchServing(t, "health.medication.dose")
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	v := dose.Version
	wantID := facet.NewID(facet.SourceRef("workbench", "workbench:med/melatonin/dose/2026-10-19T22:00+11:00"))
	page(t, "POST", base+"/entry", doseForm(v, "2026-10-19T22:00", "5"), nil)

	_, facetPage := page(t, "GET", base+"/facet?id="+wantID, nil, nil)
	if !strings.Contains(facetPage, `href="/entry?type=health.medication.dose&edit=`+wantID+`"`) ||
		!strings.Contains(facetPage, `action="/retract"`) {
		t.Fatalf("the workbench facet's page lacks Edit or Retract:\n%s", facetPage)
	}

	status, form := page(t, "GET", base+"/entry?type=health.medication.dose&edit="+wantID, nil, nil)
	if status != http.StatusOK {
		t.Fatalf("GET edit = %d: %s", status, form)
	}
	for _, name := range []string{"drug", "taken_at"} {
		if tag := inputTag(t, form, name); !strings.Contains(tag, "readonly") {
			t.Errorf("identity field %s is not locked: %s", name, tag)
		}
	}
	for name, value := range map[string]string{"drug": "melatonin", "taken_at": "2026-10-19T22:00", "amount": "5", "unit": "mg"} {
		if tag := inputTag(t, form, name); !strings.Contains(tag, `value="`+value+`"`) {
			t.Errorf("control %s = %s, want it filled with %q", name, tag, value)
		}
	}
	if tag := inputTag(t, form, "amount"); strings.Contains(tag, "readonly") {
		t.Errorf("amount is locked: %s", tag)
	}
	edit := doseForm(v, "2026-10-19T22:00", "7.5")
	edit.Set("edit", wantID)
	status, body := page(t, "POST", base+"/entry", edit, nil)
	if status != http.StatusOK || !strings.Contains(body, "superseded") || !strings.Contains(body, wantID) {
		t.Fatalf("edit save = %d, want a new version of %s:\n%s", status, wantID, body)
	}

	// A facet of another source: no buttons, and the retraction is refused.
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	var adm api.Admission
	call(t, "POST", base+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.1, weight.Version)), http.StatusOK, &adm)
	other := adm.Results[0].FacetID
	_, otherPage := page(t, "GET", base+"/facet?id="+other, nil, nil)
	if strings.Contains(otherPage, `action="/retract"`) || strings.Contains(otherPage, "&edit=") || strings.Contains(otherPage, ">Edit<") {
		t.Fatalf("a facet of another source carries a Workbench button:\n%s", otherPage)
	}
	status, body = page(t, "POST", base+"/retract", url.Values{"id": {other}}, nil)
	if status != http.StatusConflict || !strings.Contains(body, "Nothing was written") {
		t.Fatalf("retract of another source's facet = %d, want 409:\n%s", status, body)
	}

	// Retract the workbench's own, after which it is tombstoned and the buttons go.
	status, body = page(t, "POST", base+"/retract", url.Values{"id": {wantID}}, nil)
	if status != http.StatusOK || !strings.Contains(body, "retracted") {
		t.Fatalf("retract = %d, want retracted:\n%s", status, body)
	}
	res, err := fold.Read(context.Background(), d.read)
	if err != nil {
		t.Fatal(err)
	}
	if res.States[wantID].Live() {
		t.Fatalf("the retracted dose is still live: %+v", res.States[wantID])
	}
	if !res.States[other].Live() {
		t.Fatalf("the other source's facet was retracted")
	}
	_, after := page(t, "GET", base+"/facet?id="+wantID, nil, nil)
	if strings.Contains(after, `action="/retract"`) {
		t.Fatalf("a retracted facet still offers Retract")
	}
}

// The browser guard wraps the form routes: a post from another site writes nothing.
func TestTheFormRoutesRefuseACrossSitePost(t *testing.T) {
	d, base := workbenchServing(t, "health.medication.dose")
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	cross := map[string]string{"Origin": "https://untrusted.example", "Sec-Fetch-Site": "cross-site"}
	if status, _ := page(t, "POST", base+"/entry", doseForm(dose.Version, "2026-10-19T22:00", "5"), cross); status != http.StatusForbidden {
		t.Fatalf("cross-site POST /entry = %d, want 403", status)
	}
	if status, _ := page(t, "POST", base+"/retract", url.Values{"id": {"x"}}, cross); status != http.StatusForbidden {
		t.Fatalf("cross-site POST /retract = %d, want 403", status)
	}
	if w := head(t, d); w != 0 {
		t.Fatalf("W = %d after cross-site posts, want 0", w)
	}
}
