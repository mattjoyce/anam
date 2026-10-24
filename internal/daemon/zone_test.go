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
// names no zone, and a request that names UTC reads the UTC day. The record holds an
// alert at 07:30 Sydney on the 21st (20:30 UTC on the 20th) and one at 23:59 the night
// before.
func TestSelectReadsADateInTheDaemonsZone(t *testing.T) {
	cfg := importerConfig(t)
	cfg.Zone = "Australia/Sydney"
	raw, err := os.ReadFile(cfg.ImportersFile)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Replace(string(raw), "types: [health.weight, test.reading]", "types: [health.weight, test.reading, health.heart-rate-alert]", 1)
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d, base, _ := serveImporters(t, cfg)
	def, _ := d.defs.current.Load().Lookup("health.heart-rate-alert")
	env := func(at string) string {
		return fmt.Sprintf(`{"type": "health.heart-rate-alert", "definition_version": %d, "payload": {
			"source": "garmin", "kind": "hr_high_alert", "fact_time": %q, "heart_rate": 124, "threshold": 120, "unit": "bpm",
			"source_time": {"value": %q, "zone": "UTC", "basis": "source"}}}`, def.Version, at, at)
	}
	var adm api.Admission
	call(t, "POST", base+"/v1/envelopes", importerToken,
		batch(env("2026-10-20T20:30:00Z"), env("2026-10-20T12:59:00Z")), http.StatusOK, &adm)
	if len(adm.Results) != 2 || adm.Results[0].FacetID == "" || adm.Results[1].FacetID == "" {
		t.Fatalf("admitted %+v", adm.Results)
	}

	const day = "/select?type=health.heart-rate-alert&from=2026-10-21"
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
