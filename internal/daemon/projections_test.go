package daemon

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// Card 125: the operator registers a projection template over the API; a held version
// is immutable; a read selects it; a receipt served through it verifies after a restart,
// and still does once a newer version is registered.

const weightsV1 = `{"name": "weights", "version": 1, "description": "weigh-ins only",
  "types": ["health.weight"], "statuses": ["admitted", "endorsed"], "max_classification": "Sensitive",
  "from": "", "to": "", "shape": "log"}`

func TestPostProjections(t *testing.T) {
	cfg := importerConfig(t)
	d, url, stop := serveImporters(t, cfg)
	post := func(body string, want int) api.RegisteredProjection {
		t.Helper()
		var reg api.RegisteredProjection
		into := any(&reg)
		if want >= 300 {
			into = nil
		}
		call(t, "POST", url+"/v1/projections", testToken, body, want, into)
		return reg
	}

	reg := post(weightsV1, http.StatusCreated)
	if !reg.New || reg.Name != "weights" || reg.Version != 1 || !strings.HasPrefix(reg.Hash, "sha256:") {
		t.Fatalf("first registration: %+v", reg)
	}
	if again := post(strings.ReplaceAll(weightsV1, "\n", " "), http.StatusOK); again.New || again.Hash != reg.Hash {
		t.Fatalf("the same body again: %+v", again)
	}
	post(strings.Replace(weightsV1, `"endorsed"`, `"admitted"`, 1), http.StatusConflict)                       // another body at v1
	post(strings.Replace(weightsV1, `"health.weight"`, `"health.nothing"`, 1), http.StatusUnprocessableEntity) // no Definition
	post(`{"name": "weights"}`, http.StatusUnprocessableEntity)                                                // not a template
	call(t, "POST", url+"/v1/projections", importerToken, weightsV1, http.StatusForbidden, nil)                // not the operator's
	// The bundled bp again is unchanged; bp v2 over it is new, and v1 stays held.
	bp, _ := d.projections.bundled.Lookup("bp")
	if r := post(string(bp.Body), http.StatusOK); r.New {
		t.Fatalf("the bundled bp body: %+v", r)
	}
	post(strings.Replace(string(bp.Body), `"version":1`, `"version":2`, 1), http.StatusCreated)

	var list api.Projections
	call(t, "GET", url+"/v1/projections", testToken, "", http.StatusOK, &list)
	seen := map[string]api.ProjectionSummary{}
	for _, p := range list.Projections {
		seen[p.Name] = p
	}
	if w := seen["weights"]; w.Origin != api.OriginRegistered || w.Current != 1 || len(w.Versions) != 1 || w.Hash != reg.Hash {
		t.Errorf("weights listed as %+v", w)
	}
	if b := seen["bp"]; b.Origin != api.OriginRegistered || b.Current != 2 || len(b.Versions) != 2 {
		t.Errorf("bp listed as %+v", b)
	}
	if m := seen["meds"]; m.Origin != api.OriginBundled || m.Current != 3 || len(m.Versions) != 3 {
		t.Errorf("meds listed as %+v", m)
	}
	var body map[string]any
	call(t, "GET", url+"/v1/projections/weights", testToken, "", http.StatusOK, &body)
	call(t, "GET", url+"/v1/projections/bp/1", testToken, "", http.StatusOK, &body)
	if body["version"] != float64(1) {
		t.Errorf("bp v1 as held: %v", body)
	}
	call(t, "GET", url+"/v1/projections/bp/3", testToken, "", http.StatusNotFound, nil)
	call(t, "GET", url+"/v1/projections/none", testToken, "", http.StatusNotFound, nil)

	// A read selects the registered template, and a receipt pins its hash.
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.5, weight.Version)), http.StatusOK, &a)
	var pub api.Published
	call(t, "POST", url+"/v1/receipts?zone=UTC&projection=weights", testToken, "", http.StatusCreated, &pub)
	if pub.Receipt.Template != "weights" || pub.Receipt.TemplateVersion != 1 || pub.Receipt.TemplateHash != reg.Hash || len(pub.Receipt.FacetIDs) != 1 {
		t.Fatalf("published through weights: %+v", pub.Receipt)
	}

	// After a restart the registration holds, the receipt verifies, and a newer
	// version registered over it does not move the receipt's.
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	_, url, _ = serveImporters(t, cfg)
	var ver api.ReceiptVerified
	call(t, "GET", url+"/v1/receipts/"+pub.ReceiptHash+"/verify", testToken, "", http.StatusOK, &ver)
	if !ver.Result.OK() {
		t.Fatalf("verify after restart: %s", ver.Result.Summary())
	}
	post(strings.Replace(weightsV1, `"version": 1`, `"version": 2`, 1), http.StatusCreated)
	call(t, "GET", url+"/v1/receipts/"+pub.ReceiptHash+"/verify", testToken, "", http.StatusOK, &ver)
	if !ver.Result.OK() {
		t.Fatalf("verify after weights v2: %s", ver.Result.Summary())
	}
	var tl api.Timeline
	call(t, "GET", url+"/v1/timeline?zone=UTC&projection=weights", testToken, "", http.StatusOK, &tl)
	if tl.Projection == nil || tl.Projection.Version != 2 {
		t.Errorf("a read after v2 went through %+v, want v2", tl.Projection)
	}
}
