package daemon

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/facet"
)

// workbenchConfig is importerConfig with the workbench principal declared beside the
// importer: no token, and the types it may enter by hand.
func workbenchConfig(t *testing.T) (serve func() (*Daemon, string)) {
	t.Helper()
	cfg := importerConfig(t)
	f, err := os.OpenFile(cfg.ImportersFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString("  - source: workbench\n    types: [health.medication.dose]\n"); err != nil {
		t.Fatal(err)
	}
	return func() (*Daemon, string) {
		d, url, _ := serveImporters(t, cfg)
		return d, url
	}
}

// doseEntry is an entry body for a melatonin dose at the minute taken, with no id.
func doseEntry(version int, taken string, amount float64) string {
	return fmt.Sprintf(`{"type": "health.medication.dose", "definition_version": %d,
		"payload": {"drug": "melatonin", "amount": %v, "unit": "mg", "taken_at": %q, "reason": "to help sleep", "source_time": {"value": "%[3]s", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}}}`,
		version, amount, taken)
}

// Card 188 slice 2: a dose entered through the route lands under the workbench with
// the id the Definition composes, entering it again is a no-op, a changed dose is a
// new version of the same facet, and status lists the workbench once it has written.
func TestPostEntryAdmitsADoseAsTheWorkbench(t *testing.T) {
	serve := workbenchConfig(t)
	d, url := serve()
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	v := dose.Version

	var got api.Entry
	call(t, "POST", url+"/v1/entry", testToken, doseEntry(v, "2026-10-19 22:00", 5), http.StatusOK, &got)
	wantID := facet.NewID(facet.SourceRef("workbench", "workbench:med/melatonin/dose/2026-10-19T22:00+11:00"))
	if got.Outcome != "admitted" || got.FacetID != wantID || got.ArtifactVersion != 1 || got.Seq != 1 || got.W != 1 {
		t.Fatalf("first entry = %+v, want admitted %s at version 1, seq 1, W 1", got, wantID)
	}
	f, err := d.read.CurrentVersion(context.Background(), got.FacetID)
	if err != nil {
		t.Fatal(err)
	}
	if f.Provenance.Source != "workbench" || f.Provenance.Injector != "workbench" {
		t.Fatalf("provenance = %+v, want source and injector workbench", f.Provenance)
	}
	if want := facet.SourceRef("workbench", "workbench:med/melatonin/dose/2026-10-19T22:00+11:00"); f.Provenance.SourceRef != want {
		t.Fatalf("source_ref = %q, want %q", f.Provenance.SourceRef, want)
	}

	// The same entry again holds the same facet and writes nothing.
	got = api.Entry{}
	call(t, "POST", url+"/v1/entry", testToken, doseEntry(v, "2026-10-19 22:00", 5), http.StatusOK, &got)
	if got.Outcome != "no-op" || got.FacetID != wantID || got.Seq != 0 || got.W != 1 {
		t.Fatalf("second entry = %+v, want no-op on %s with no seq, W 1", got, wantID)
	}
	if w := head(t, d); w != 1 {
		t.Fatalf("W = %d after a repeat, want 1", w)
	}

	// A changed amount at the same minute is a new version of the same facet.
	got = api.Entry{}
	call(t, "POST", url+"/v1/entry", testToken, doseEntry(v, "2026-10-19 22:00", 10), http.StatusOK, &got)
	if got.Outcome != "superseded" || got.FacetID != wantID || got.ArtifactVersion != 2 || got.Seq != 2 {
		t.Fatalf("changed entry = %+v, want superseded to version 2, seq 2", got)
	}

	var st api.Status
	call(t, "GET", url+"/v1/status", testToken, "", http.StatusOK, &st)
	var seen bool
	for _, s := range st.Sources {
		if s.Source == "workbench" && s.LastAdmissionAt != "" {
			seen = true
		}
	}
	if !seen {
		t.Fatalf("status sources = %+v, want workbench with a last admission", st.Sources)
	}
}

// The route refuses what an importer would be refused, and says so in the gate's
// words: a type outside the workbench's types, a payload the Definition refuses, an
// id that disagrees with the rule, and a form drawn from another Definition version.
func TestPostEntryRefusesAsAnImporterWouldBe(t *testing.T) {
	serve := workbenchConfig(t)
	d, url := serve()
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	v := dose.Version

	cases := []struct {
		name, body string
		status     int
		reason     string
	}{
		{"a type outside the workbench's types",
			`{"type": "health.weight", "definition_version": 3, "payload": {"id": "w", "date": "2026-10-09 07:30", "kg": 80}}`,
			http.StatusUnprocessableEntity, "source not authorized"},
		{"a payload the Definition refuses",
			fmt.Sprintf(`{"type": "health.medication.dose", "definition_version": %d, "payload": {"drug": "melatonin", "amount": "five", "unit": "mg", "taken_at": "2026-10-19 22:00", "source_time": {"value": "2026-10-19 22:00", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}}}`, v),
			http.StatusUnprocessableEntity, "not compliant"},
		{"an id that disagrees with the rule",
			fmt.Sprintf(`{"type": "health.medication.dose", "definition_version": %d, "payload": {"id": "workbench:med/melatonin/dose/other", "drug": "melatonin", "amount": 5, "unit": "mg", "taken_at": "2026-10-19 22:00", "source_time": {"value": "2026-10-19 22:00", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}}}`, v),
			http.StatusUnprocessableEntity, "identity"},
		{"a definition version that is not in force", doseEntry(v+1, "2026-10-19 22:00", 5),
			http.StatusConflict, "another definition version"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := postAs(t, url+"/v1/entry", c.body, nil)
			if status != c.status || !strings.Contains(body, c.reason) {
				t.Fatalf("%d %s, want %d naming %q", status, body, c.status, c.reason)
			}
			var r api.Refusal
			call(t, "POST", url+"/v1/entry", testToken, c.body, c.status, &r)
			if len(r.Refused) != 1 || r.Refused[0].Reason == "" || !strings.Contains(r.Error, r.Refused[0].Reason) {
				t.Fatalf("refusal = %+v, want the gate's reason, verbatim", r)
			}
		})
	}
	if w := head(t, d); w != 0 {
		t.Fatalf("W = %d after refusals, want 0", w)
	}
	if st := d.sources.statuses(); len(st) != 0 {
		t.Fatalf("a refused entry marked the workbench as seen: %+v", st)
	}
}

// The body is the type, the version and a payload object, and nothing else: the client
// does not choose a source, an injector or an id, and a body that is not one bounded
// value is a 400.
func TestPostEntryTakesOnlyTheThreeFields(t *testing.T) {
	serve := workbenchConfig(t)
	d, url := serve()
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	v := dose.Version
	payload := `{"drug": "melatonin", "amount": 5, "unit": "mg", "taken_at": "2026-10-19 22:00", "source_time": {"value": "2026-10-19 22:00", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}}`

	for name, body := range map[string]string{
		"a source":                  fmt.Sprintf(`{"type": "health.medication.dose", "definition_version": %d, "source": "garmin", "payload": %s}`, v, payload),
		"an injector":               fmt.Sprintf(`{"type": "health.medication.dose", "definition_version": %d, "injector": "x", "payload": %s}`, v, payload),
		"an id outside the payload": fmt.Sprintf(`{"type": "health.medication.dose", "definition_version": %d, "id": "x", "payload": %s}`, v, payload),
		"no type":                   fmt.Sprintf(`{"definition_version": %d, "payload": %s}`, v, payload),
		"no definition version":     `{"type": "health.medication.dose", "payload": ` + payload + `}`,
		"no payload":                fmt.Sprintf(`{"type": "health.medication.dose", "definition_version": %d}`, v),
		"a payload that is not an object": fmt.Sprintf(
			`{"type": "health.medication.dose", "definition_version": %d, "payload": [1]}`, v),
		"a second value": doseEntry(v, "2026-10-19 22:00", 5) + ` {}`,
		"not JSON":       `entry`,
	} {
		t.Run(name, func(t *testing.T) {
			call(t, "POST", url+"/v1/entry", testToken, body, http.StatusBadRequest, nil)
		})
	}
	huge := fmt.Sprintf(`{"type": "health.medication.dose", "definition_version": %d, "payload": {"reason": %q}}`,
		v, strings.Repeat("x", d.cfg.MaxPayloadBytes+entryHeadroom))
	call(t, "POST", url+"/v1/entry", testToken, huge, http.StatusRequestEntityTooLarge, nil)
	if w := head(t, d); w != 0 {
		t.Fatalf("W = %d after refused bodies, want 0", w)
	}
}

// Only the operator may enter, and a browser on another site may not: the operator's
// token is the browser's ambient credential, and the guard is what keeps a foreign
// page from spending it.
func TestPostEntryIsTheOperatorsAndNotAForeignPages(t *testing.T) {
	serve := workbenchConfig(t)
	d, url := serve()
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	body := doseEntry(dose.Version, "2026-10-19 22:00", 5)

	call(t, "POST", url+"/v1/entry", importerToken, body, http.StatusForbidden, nil)
	call(t, "POST", url+"/v1/entry", "", body, http.StatusUnauthorized, nil)
	for name, headers := range map[string]map[string]string{
		"a cross-site fetch":     {"Sec-Fetch-Site": "cross-site"},
		"another port's origin":  {"Origin": "https://untrusted.example"},
		"a same-site other page": {"Sec-Fetch-Site": "same-site", "Origin": "https://untrusted.example"},
	} {
		t.Run(name, func(t *testing.T) {
			if status, answer := postAs(t, url+"/v1/entry", body, headers); status != http.StatusForbidden {
				t.Fatalf("%d %s, want 403", status, answer)
			}
		})
	}
	if w := head(t, d); w != 0 {
		t.Fatalf("W = %d after refused requests, want 0", w)
	}
	// The same page's own request, from its own origin, is let through.
	status, answer := postAs(t, url+"/v1/entry", body, map[string]string{"Sec-Fetch-Site": "same-origin", "Origin": url})
	if status != http.StatusOK {
		t.Fatalf("same-origin: %d %s", status, answer)
	}
}

// A daemon whose importers file does not declare the workbench answers 404 and says
// what is missing, and so does one with no importers file at all.
func TestPostEntryWithoutAWorkbenchPrincipal(t *testing.T) {
	cfg := importerConfig(t)
	d, url, _ := serveImporters(t, cfg)
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")
	status, answer := postAs(t, url+"/v1/entry", doseEntry(dose.Version, "2026-10-19 22:00", 5), nil)
	if status != http.StatusNotFound || !strings.Contains(answer, "declares no workbench principal") {
		t.Fatalf("%d %s, want 404 saying the importers file declares no workbench principal", status, answer)
	}
	if w := head(t, d); w != 0 {
		t.Fatalf("W = %d, want 0", w)
	}
}
