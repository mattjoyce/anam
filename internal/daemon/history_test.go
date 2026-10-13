package daemon

import (
	"net/http"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// Card 111: a daemon on a store built from nothing still publishes every version a
// facet may have been admitted under, and an importer registering an older version of
// a type it took over finds it held rather than refused.
func TestDefinitionHistorySurvivesAFreshStore(t *testing.T) {
	cfg := importerConfig(t)
	_, url, _ := serveImporters(t, cfg)

	var list api.Definitions
	call(t, "GET", url+"/v1/definitions", testToken, "", http.StatusOK, &list)
	for _, s := range list.Definitions {
		if s.Type == "health.blood-pressure" && len(s.Versions) != 5 {
			t.Errorf("health.blood-pressure versions = %v, want 1 to 5", s.Versions)
		}
	}
	var body map[string]any
	call(t, "GET", url+"/v1/definitions/health.blood-pressure/1", testToken, "", http.StatusOK, &body)
	call(t, "GET", url+"/v1/definitions/health.blood-pressure/4", testToken, "", http.StatusOK, &body)
	call(t, "GET", url+"/v1/definitions/health.weight/1", testToken, "", http.StatusOK, &body)
	call(t, "GET", url+"/v1/definitions/health.blood-pressure/9", testToken, "", http.StatusNotFound, nil)
}
