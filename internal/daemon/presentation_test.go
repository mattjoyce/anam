package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/presentation"
)

func TestPresentationFailureBeforeStoresAndRecovery(t *testing.T) {
	t.Parallel()
	cfg := newConfig(t)
	// These unusable paths prove validation precedes catalog, token, store and listener IO.
	prior := cfg
	cfg.LedgerPath = filepath.Join(t.TempDir(), "absent", "ledger.db")
	cfg.DBPath = filepath.Join(t.TempDir(), "absent", "tracking.db")
	cfg.TokenFile = filepath.Join(t.TempDir(), "absent-token")
	cfg.Inspector = presentation.Spec{Families: []presentation.Family{{ID: "other"}}}
	if err := cfg.Validate(); !errors.Is(err, config.ErrInvalid) || !strings.Contains(err.Error(), "identifier") {
		t.Fatalf("Validate = %v", err)
	}
	if d, err := Open(context.Background(), cfg, Options{}); d != nil || !errors.Is(err, config.ErrInvalid) || !strings.Contains(err.Error(), "identifier") {
		t.Fatalf("Open = %v, %v", d, err)
	}
	for _, file := range []string{cfg.LedgerPath, cfg.DBPath} {
		if _, err := os.Stat(filepath.Dir(file)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("failed start created store directory: %v", err)
		}
	}
	// Recovery accepts the prior policy and renders its neutral defaults.
	d := open(t, prior)
	defer func() { _ = d.closeStores() }()
	w := httptest.NewRecorder()
	d.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/?view=whole", nil))
	if w.Code != http.StatusOK || d.cfg.Presentation().Hue("health") != "hx" {
		t.Fatal("prior config did not recover")
	}
}

func TestPresentationIdentityAndReceipts(t *testing.T) {
	t.Parallel()
	cfg := readerConfig(t)
	d := open(t, cfg)
	firstV, firstBodies, firstHistory := d.catalog.SetVersion(), d.catalog.Manifests(), d.catalog.History()
	srv := httptest.NewServer(d.Handler())
	var first api.Published
	call(t, "GET", srv.URL+timelineMeasures, recorderToken, "", http.StatusOK, &first)
	var verified api.ReceiptVerified
	call(t, "GET", srv.URL+"/v1/receipts/"+first.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &verified)
	srv.Close()
	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}
	hue := 4
	cfg.Inspector = presentation.Spec{Domains: []presentation.DomainSpec{{ID: "test", Hue: &hue}}, Families: []presentation.Family{{ID: "measures", Label: "Custom readings"}}, Types: []presentation.Override{{Type: "test.measure", Shape: "star"}}}
	d2 := open(t, cfg)
	defer func() { _ = d2.closeStores() }()
	if d2.catalog.SetVersion() != firstV || !reflect.DeepEqual(d2.catalog.Manifests(), firstBodies) || !reflect.DeepEqual(d2.catalog.History(), firstHistory) {
		t.Fatal("presentation changed catalog identity or body bytes")
	}
	srv2 := httptest.NewServer(d2.Handler())
	defer srv2.Close()
	var second api.Published
	call(t, "GET", srv2.URL+timelineMeasures, plainToken, "", http.StatusOK, &second)
	// The response timestamp/receipt identifies the request; its fact value stays identical at W.
	if first.W != second.W || !reflect.DeepEqual(first.Timeline, second.Timeline) {
		t.Fatal("presentation changed fixed-watermark API facts")
	}
	var after api.ReceiptVerified
	call(t, "GET", srv2.URL+"/v1/receipts/"+first.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &after)
	if !verified.Result.OK() || !reflect.DeepEqual(verified.Result, after.Result) {
		t.Fatal("presentation changed receipt verification")
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/?view=whole&open=test", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	d2.Handler().ServeHTTP(w, req)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `class="h4"`) {
		t.Fatal("daemon ignored compiled presentation")
	}
	hue = 7
	cfg.Inspector.Types[0].Shape = "cross"
	w = httptest.NewRecorder()
	d2.Handler().ServeHTTP(w, req)
	if !strings.Contains(w.Body.String(), `class="h4"`) {
		t.Fatal("caller mutated daemon policy")
	}
}
