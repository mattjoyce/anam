package daemon

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/projection"
)

// Card 125: the operator registers a projection template over the API; a held version
// is immutable; a read selects it; a receipt served through it verifies after a restart,
// and still does once a newer version is registered.

const weightsV1 = `{"name": "weights", "version": 1, "description": "weigh-ins only",
  "types": ["health.weight"], "statuses": ["admitted", "endorsed"], "max_classification": "Sensitive",
  "from": "", "to": "", "shape": "log"}`

func TestPostProjections(t *testing.T) {
	t.Parallel()
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
	if m := seen["meds"]; m.Origin != api.OriginBundled || m.Current != 4 || len(m.Versions) != 4 {
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

// Card 180: the operator's default_projection is advertised on status and on the
// projections list, applied to nothing, and a name that is neither bundled nor
// registered stops the daemon at start.
func TestDefaultProjectionAdvertised(t *testing.T) {
	t.Parallel()
	t.Run("none set", func(t *testing.T) {
		_, url, _ := serveImporters(t, importerConfig(t))
		var raw map[string]any
		call(t, "GET", url+"/v1/status", testToken, "", http.StatusOK, &raw)
		if _, ok := raw["default_projection"]; ok {
			t.Errorf("status advertises a default with none set: %v", raw)
		}
		raw = nil
		call(t, "GET", url+"/v1/projections", testToken, "", http.StatusOK, &raw)
		if _, ok := raw["default"]; ok {
			t.Errorf("projections advertises a default with none set: %v", raw)
		}
	})

	t.Run("bundled", func(t *testing.T) {
		cfg := importerConfig(t)
		cfg.DefaultProjection = "bp"
		_, url, _ := serveImporters(t, cfg)
		var st api.Status
		call(t, "GET", url+"/v1/status", testToken, "", http.StatusOK, &st)
		if st.DefaultProjection != "bp" {
			t.Errorf("status default_projection = %q, want bp", st.DefaultProjection)
		}
		var list api.Projections
		call(t, "GET", url+"/v1/projections", testToken, "", http.StatusOK, &list)
		if list.Default != "bp" {
			t.Errorf("projections default = %q, want bp", list.Default)
		}
		// Advertised, not applied: a read naming no projection serves no template.
		var tl api.Timeline
		call(t, "GET", url+"/v1/timeline?zone=UTC", testToken, "", http.StatusOK, &tl)
		if tl.Projection != nil {
			t.Errorf("a read naming no projection went through %+v", tl.Projection)
		}
	})

	t.Run("registered, then kept across a restart", func(t *testing.T) {
		cfg := importerConfig(t)
		_, url, stop := serveImporters(t, cfg)
		call(t, "POST", url+"/v1/projections", testToken, weightsV1, http.StatusCreated, nil)
		if err := stop(); err != nil {
			t.Fatalf("stop: %v", err)
		}
		cfg.DefaultProjection = "weights"
		_, url, _ = serveImporters(t, cfg)
		var st api.Status
		call(t, "GET", url+"/v1/status", testToken, "", http.StatusOK, &st)
		if st.DefaultProjection != "weights" {
			t.Errorf("status default_projection = %q, want weights", st.DefaultProjection)
		}
	})

	t.Run("a name that is nothing", func(t *testing.T) {
		cfg := importerConfig(t)
		cfg.DefaultProjection = "no-such-projection"
		_, err := Open(context.Background(), cfg, Options{})
		if !errors.Is(err, projection.ErrUnknown) {
			t.Fatalf("Open with an unknown default_projection = %v, want projection.ErrUnknown", err)
		}
	})
}
