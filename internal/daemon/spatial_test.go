package daemon

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
)

func TestSpatialEmptyCatalogThenRegistration(t *testing.T) {
	t.Parallel()
	cfg := importerConfig(t)
	cfg.DefinitionsDir, cfg.ProjectionsDir = "", ""
	cfg.Inspector.TrackType = "test.registered"
	d, base := serveBuilt(t, cfg)
	catalogtest.RequireEmpty(t, d.catalog)
	if body := operatorPage(t, base, "/location"); !strings.Contains(body, "no test.registered facets are live") {
		t.Fatal("enabled empty catalog did not render")
	}
	manifest := `{"meta":{"facet_type":"test.registered","definition_version":1,"classification":"Sensitive","validation_mode":"strict","source_schema":"test/hour","kind":"measure","family":"location","identity_key":["/id"],"time_semantics":{"fact_time":{"pointer":"/hour","layout":"2006-01-02T15:04:05Z07:00","zone":"UTC"}}},"schema":{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"required":["id","hour","device","points"],"properties":{"id":{"type":"string","minLength":1},"hour":{"type":"string","minLength":1},"device":{"type":"string","minLength":1},"points":{"type":"array","items":{"type":"object","additionalProperties":false,"required":["t","lat","lon"],"properties":{"t":{"type":"string","minLength":1},"lat":{"type":"number"},"lon":{"type":"number"},"acc":{"type":"number"}}}}}}}`
	call(t, "POST", base+"/v1/definitions", importerToken, manifest, http.StatusCreated, nil)
	payload := `{"id":"phone-hour","hour":"2026-10-12T02:00:00Z","device":"phone","points":[{"t":"2026-10-12T02:05:00Z","lat":-33.8,"lon":151.2,"acc":10}]}`
	call(t, "POST", base+"/v1/envelopes", importerToken, batch(`{"type":"test.registered","definition_version":1,"payload":`+payload+`}`), http.StatusOK, nil)
	if body := operatorPage(t, base, "/location"); !strings.Contains(body, "1 fix over 1 hour") || !strings.Contains(body, `class="fix h2"`) {
		t.Fatal("later registration/admission invisible")
	}
}
