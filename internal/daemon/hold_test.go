package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
)

// secondToken is a second importer's token, so a test can hold one source and see
// that another carries on.
const secondToken = "second-hold-token-0123456789abcdef01234"

// holdConfig is importerConfig with a second importer and the workbench declared
// beside them.
func holdConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := importerConfig(t)
	first, second := sha256.Sum256([]byte(importerToken)), sha256.Sum256([]byte(secondToken))
	body := fmt.Sprintf("importers:\n  - source: test-import\n    token_sha256: %s\n    types: [health.weight, test.reading]\n"+
		"  - source: second-import\n    token_sha256: %s\n    types: [health.weight]\n"+
		"  - source: workbench\n    types: [health.medication.dose]\n",
		hex.EncodeToString(first[:]), hex.EncodeToString(second[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

// hold answers a hold request and returns the status, the body and the Retry-After.
func holdCall(t *testing.T, method, url, token, body string) (int, string, string) {
	t.Helper()
	req, err := http.NewRequest(method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw), resp.Header.Get("Retry-After")
}

// A held source admits nothing: its envelopes and its Definition registrations are
// refused with 503 and a Retry-After, and the body names the hold. Another source
// carries on, the operator's own writes carry on, and the workbench is never held.
func TestHoldRefusesAHeldSourceAndOnlyThatSource(t *testing.T) {
	d, url, _ := serveImporters(t, holdConfig(t))
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	dose, _ := d.defs.current.Load().Lookup("health.medication.dose")

	var held api.Holds
	call(t, "POST", url+"/v1/hold", testToken, `{"sources": ["test-import"]}`, http.StatusOK, &held)
	if len(held.Held) != 1 || held.Held[0] != "test-import" {
		t.Fatalf("hold = %+v, want test-import held", held)
	}

	// The held source's batch is refused, and the answer names the hold.
	status, body, retry := holdCall(t, "POST", url+"/v1/envelopes", importerToken,
		batch(weighIn("w-held", 80.5, weight.Version)))
	if status != http.StatusServiceUnavailable {
		t.Fatalf("a held source's batch: %d %s, want 503", status, body)
	}
	if retry == "" {
		t.Error("a held source's batch: no Retry-After header")
	}
	if !strings.Contains(body, "held") || !strings.Contains(body, "test-import") {
		t.Errorf("a held source's batch: %s, want a body naming the hold", body)
	}

	// Its Definition registration is refused too.
	status, body, _ = holdCall(t, "POST", url+"/v1/definitions", importerToken, readingManifest)
	if status != http.StatusServiceUnavailable || !strings.Contains(body, "held") {
		t.Fatalf("a held source's registration: %d %s, want 503 naming the hold", status, body)
	}

	// Another source carries on.
	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", secondToken, batch(weighIn("w-free", 70, weight.Version)), http.StatusOK, &a)
	if a.Changed != 1 {
		t.Fatalf("another source's batch = %+v, want one facet admitted", a)
	}

	// The workbench is never held: a hand entry goes in while every source is held.
	call(t, "POST", url+"/v1/hold", testToken, `{"all": true}`, http.StatusOK, &held)
	if len(held.Held) != 2 {
		t.Fatalf("a global hold = %+v, want two holds held", held)
	}
	var entry api.Entry
	call(t, "POST", url+"/v1/entry", testToken, doseEntry(dose.Version, "2026-10-19 22:00", 5), http.StatusOK, &entry)
	if entry.Outcome != "admitted" {
		t.Fatalf("an entry under a global hold = %+v, want admitted", entry)
	}

	// The held source is still refused under the global hold.
	status, body, _ = holdCall(t, "POST", url+"/v1/envelopes", importerToken,
		batch(weighIn("w-held-2", 80.5, weight.Version)))
	if status != http.StatusServiceUnavailable || !strings.Contains(body, "held") {
		t.Fatalf("a held source's batch under a global hold: %d %s, want 503 naming the hold", status, body)
	}

	// The status says what is held.
	var st api.Status
	call(t, "GET", url+"/v1/status", testToken, "", http.StatusOK, &st)
	if len(st.Held) != 2 || st.Held[0] != "*" || st.Held[1] != "test-import" {
		t.Fatalf("status holds = %+v, want * and test-import", st.Held)
	}

	// Releasing admits again.
	call(t, "DELETE", url+"/v1/hold", testToken, `{"all": true}`, http.StatusOK, &held)
	if len(held.Held) != 0 {
		t.Fatalf("a release = %+v, want no holds held", held)
	}
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-free-2", 60, weight.Version)), http.StatusOK, &a)
	if a.Changed != 1 {
		t.Fatalf("a released source's batch = %+v, want one facet admitted", a)
	}
}

// A hold is the operator's alone: an importer's token is refused, and so is none.
func TestHoldIsTheOperatorsAlone(t *testing.T) {
	_, url, _ := serveImporters(t, holdConfig(t))
	for name, tok := range map[string]string{"an importer's token": importerToken, "no token": ""} {
		t.Run(name, func(t *testing.T) {
			want := http.StatusForbidden
			if tok == "" {
				want = http.StatusUnauthorized
			}
			call(t, "POST", url+"/v1/hold", tok, `{"sources": ["test-import"]}`, want, nil)
			call(t, "DELETE", url+"/v1/hold", tok, `{"sources": ["test-import"]}`, want, nil)
		})
	}
}

// The workbench is never held: a hold names an importer's source, and a hand entry is
// the operator's own.
func TestHoldRefusesTheWorkbench(t *testing.T) {
	_, url, _ := serveImporters(t, holdConfig(t))
	status, body, _ := holdCall(t, "POST", url+"/v1/hold", testToken, `{"sources": ["workbench"]}`)
	if status != http.StatusBadRequest || !strings.Contains(body, "never held") {
		t.Fatalf("a hold on the workbench: %d %s, want 400 saying it is never held", status, body)
	}
}

// A hold needs something to hold: an empty request is refused, and holding a source
// twice keeps the hold it has.
func TestHoldNeedsASourceAndKeepsTheOneItHas(t *testing.T) {
	_, url, _ := serveImporters(t, holdConfig(t))
	call(t, "POST", url+"/v1/hold", testToken, `{}`, http.StatusBadRequest, nil)

	var held api.Holds
	call(t, "POST", url+"/v1/hold", testToken, `{"sources": ["test-import"]}`, http.StatusOK, &held)
	call(t, "POST", url+"/v1/hold", testToken, `{"sources": ["test-import"]}`, http.StatusOK, &held)
	if len(held.Held) != 1 || held.Held[0] != "test-import" {
		t.Fatalf("a second hold = %+v, want test-import held", held)
	}
}

// A hold is kept in the operational store, so it outlives the daemon: a daemon that
// starts on the same store finds the hold and keeps refusing the source.
func TestHoldSurvivesARestart(t *testing.T) {
	cfg := holdConfig(t)
	d, url, _ := serveImporters(t, cfg)
	call(t, "POST", url+"/v1/hold", testToken, `{"sources": ["test-import"]}`, http.StatusOK, nil)
	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}

	again := open(t, cfg)
	start(t, again) // the write worker and the start-up catch-up run
	srv := httptest.NewServer(again.Handler())
	defer srv.Close()
	var st api.Status
	call(t, "GET", srv.URL+"/v1/status", testToken, "", http.StatusOK, &st)
	if len(st.Held) != 1 || st.Held[0] != "test-import" {
		t.Fatalf("status holds after a restart = %+v, want test-import held", st.Held)
	}
	weight, _ := again.defs.current.Load().Lookup("health.weight")
	status, body, _ := holdCall(t, "POST", srv.URL+"/v1/envelopes", importerToken,
		batch(weighIn("w-restart", 80.5, weight.Version)))
	if status != http.StatusServiceUnavailable || !strings.Contains(body, "held") {
		t.Fatalf("a held source's batch after a restart: %d %s, want 503 naming the hold", status, body)
	}
}
