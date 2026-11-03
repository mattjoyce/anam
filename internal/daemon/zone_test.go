package daemon

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// The card's own case through the live daemon (card 236): a daemon configured for
// Sydney reads "from 21 Oct" on the select page as the Sydney day when the request
// names no zone, and a request that names UTC reads the UTC day. The record holds a
// test.labelled, read in UTC, at 07:30 Sydney on the 21st (20:30 UTC on the 20th) and
// one at 23:59 the night before.
func TestSelectReadsADateInTheDaemonsZone(t *testing.T) {
	t.Parallel()
	cfg := importerConfig(t)
	cfg.Zone = "Australia/Sydney"
	raw, err := os.ReadFile(cfg.ImportersFile)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(raw), "types: [test.reading, test.registered]", "types: [test.reading, test.registered, test.labelled]", 1)
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d, base, _ := serveImporters(t, cfg)
	def, _ := d.defs.current.Load().Lookup("test.labelled")
	env := func(at string) string {
		return fmt.Sprintf(`{"type": "test.labelled", "definition_version": %d, "payload": {
			"id": %q, "at": %q, "kind": "other", "text": "a high heart rate"}}`, def.Version, "alert "+at, at)
	}
	var adm api.Admission
	call(t, "POST", base+"/v1/envelopes", importerToken,
		batch(env("2026-10-20 20:30"), env("2026-10-20 12:59")), http.StatusOK, &adm)
	if len(adm.Results) != 2 || adm.Results[0].FacetID == "" || adm.Results[1].FacetID == "" {
		t.Fatalf("admitted %+v", adm.Results)
	}

	const day = "/select?type=test.labelled&from=2026-10-21"
	for _, c := range []struct{ name, query, matched, zone string }{
		{"the daemon's zone", "", "1", "Australia/Sydney"},
		{"named UTC", "&zone=UTC", "0", "UTC"}, // the UTC day holds neither alert
		{"named Sydney", "&zone=Australia/Sydney", "1", "Australia/Sydney"},
	} {
		code, got := page(t, "GET", base+day+c.query, nil, nil)
		if code != http.StatusOK {
			t.Fatalf("%s: GET = %d\n%s", c.name, code, got)
		}
		if want := `<strong id="matched">` + c.matched + `</strong>`; !strings.Contains(got, want) {
			t.Errorf("%s: want %s:\n%s", c.name, want, got)
		}
		if want := "% dates read in " + c.zone; !strings.Contains(got, want) {
			t.Errorf("%s: the program does not say %q:\n%s", c.name, want, got)
		}
	}
	if code, got := page(t, "GET", base+day+"&zone=Mars/Olympus", nil, nil); code != http.StatusBadRequest || !strings.Contains(got, "Mars/Olympus") {
		t.Errorf("an unknown zone = %d %q, want 400 naming it", code, got)
	}
}
