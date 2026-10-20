package daemon

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// retractBody is a retraction's body for source.
func retractBody(source string) string { return fmt.Sprintf(`{"source": %q}`, source) }

// postAs sends body to url as the operator with extra headers, and returns the status
// and the answer.
func postAs(t *testing.T, url, body string, headers map[string]string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
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

// Card 188 slice 3: the operator tombstones one facet of a source by name. The
// route applies the plan the CLI applies: a facet of another source, a facet that is
// not live and a source reconciled by an importer's snapshot are each 409 and write
// nothing; an unknown facet is 404; a browser on another site is 403.
func TestPostFacetRetract(t *testing.T) {
	d, url, _ := serveImporters(t, importerConfig(t))
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	v := weight.Version
	var adm api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken,
		batch(weighIn("w-1", 80.1, v), weighIn("w-2", 80.2, v)), http.StatusOK, &adm)
	first, second := adm.Results[0].FacetID, adm.Results[1].FacetID
	at := adm.W
	route := func(id string) string { return url + "/v1/facets/" + id + "/retract" }
	unchanged := func(step string) {
		t.Helper()
		if w := head(t, d); w != at {
			t.Fatalf("%s: W moved from %d to %d", step, at, w)
		}
	}

	// A browser on another site is refused before anything is read or written.
	status, body := postAs(t, route(first), retractBody("test-import"),
		map[string]string{"Origin": "https://untrusted.example", "Sec-Fetch-Site": "cross-site"})
	if status != http.StatusForbidden {
		t.Fatalf("cross-site: %d %s", status, body)
	}
	unchanged("cross-site")

	// Bodies that are not a source.
	call(t, "POST", route(first), testToken, `{}`, http.StatusBadRequest, nil)
	call(t, "POST", route(first), testToken, `{"source": "test-import", "facet": "x"}`, http.StatusBadRequest, nil)
	call(t, "POST", route(first), testToken, retractBody("test-import")+`{}`, http.StatusBadRequest, nil)
	// A facet the ledger does not hold.
	call(t, "POST", route("no-such-facet"), testToken, retractBody("test-import"), http.StatusNotFound, nil)
	// Only the operator.
	call(t, "POST", route(first), importerToken, retractBody("test-import"), http.StatusForbidden, nil)

	// The wrong source.
	status, body = postAs(t, route(first), retractBody("workbench"), nil)
	if status != http.StatusConflict || !strings.Contains(body, "another source") {
		t.Fatalf("wrong source: %d %s", status, body)
	}
	unchanged("wrong source")

	// A declared importer with no complete snapshot is retractable; the tombstone is the
	// next position, and the answer says so.
	var got api.Retraction
	call(t, "POST", route(first), testToken, retractBody("test-import"), http.StatusOK, &got)
	want := api.Retraction{FacetID: first, Source: "test-import", Type: "health.weight", ArtifactVersion: 1,
		Outcome: "tombstoned", Seq: at + 1, W: at + 1}
	if got != want {
		t.Fatalf("retraction = %+v, want %+v", got, want)
	}
	if w := head(t, d); w != at+1 {
		t.Fatalf("W = %d, want %d", w, at+1)
	}
	at++

	// Not live any more.
	status, body = postAs(t, route(first), retractBody("test-import"), nil)
	if status != http.StatusConflict || !strings.Contains(body, "not live") {
		t.Fatalf("tombstoned facet: %d %s", status, body)
	}
	unchanged("tombstoned facet")

	// Once the importer completes a snapshot of its source, its next resend reconciles
	// it, and a hand retraction is refused.
	snap := `{"snapshot": {"id": "run-1", "part": 1, "parts": 1, "types": ["health.weight"]}, "envelopes": [` +
		weighIn("w-2", 80.2, v) + `]}`
	call(t, "POST", url+"/v1/envelopes", importerToken, snap, http.StatusOK, &adm)
	at = adm.W
	status, body = postAs(t, route(second), retractBody("test-import"), nil)
	if status != http.StatusConflict || !strings.Contains(body, "reconciled by an importer's snapshot") {
		t.Fatalf("reconciled source: %d %s", status, body)
	}
	unchanged("reconciled source")
}
