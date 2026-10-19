package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// seedReadings admits two blood-pressure readings through the real gate before the
// daemon opens the ledger.
func seedReadings(t *testing.T, cfg config.Config) {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Open(ctx, cfg.LedgerPath, cfg.PersonID)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = l.Close() }()
	defs, err := def.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	g := gate.New(l, defs, gate.StaticScopes{"records": {"health.blood-pressure"}}, clock.Real{}, cfg.PersonID,
		gate.Limits{MaxPayloadBytes: config.DefaultMaxPayloadBytes, MaxRetries: config.DefaultCommitRetries})
	for _, p := range []string{
		`{"id": "2026-1-24 06:36:00:1", "date_excel": "2026-1-24 06:36:00", "reading": "1", "systolic_blood_pressure": {"value": 114.0, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 80.0, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-01-24T06:36:00+11:00"}, "pulse": "70.0"}`,
		`{"id": "2026-2-10 05:04:00:1", "date_excel": "2026-2-10 05:04:00", "reading": "1", "systolic_blood_pressure": {"value": 102.0, "unit": "mmHg"}, "diastolic_blood_pressure": {"value": 72.0, "unit": "mmHg"}, "effective_time_frame": {"date_time": "2026-02-10T05:04:00+11:00"}, "pulse": "68.0"}`,
	} {
		env := facet.Envelope{Type: "health.blood-pressure", Source: "records", Injector: "records", Payload: []byte(p), AssertedPerson: cfg.PersonID}
		b, err := g.Resolve(env)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := g.Commit(ctx, env, b); err != nil {
			t.Fatal(err)
		}
	}
}

func postJSON(t *testing.T, url string, body any, into any) int {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b)) // #nosec G107 -- a loopback test server
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(into); err != nil {
		t.Fatalf("POST %s: decode: %v", url, err)
	}
	return resp.StatusCode
}

// GET /v1/query is the catalogue; POST /v1/query answers with a receipt, refuses a
// bad program as the reader's error, and bounds the derivation by max_read_bytes
// with the query's narrowings.
func TestQueryRoutes(t *testing.T) {
	cfg := newConfig(t)
	seedReadings(t, cfg)
	d := open(t, cfg)
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	var cat api.QueryCatalogue
	getJSON(t, srv.URL+"/v1/query", http.StatusOK, &cat)
	if len(cat.Relations) != 4 || cat.Rules.Name != "anam" || len(cat.Types) == 0 {
		t.Errorf("catalogue = %d relations, rules %s, %d types", len(cat.Relations), cat.Rules.Name, len(cat.Types))
	}

	req := api.QueryRequest{Projection: "bp", Relation: "high",
		Text: `high(T, S) :- entry(E, "health.blood-pressure", T, _, _, _, _), attr(E, "/systolic_blood_pressure/value", S), S > 110.`}
	var a api.QueryAnswer
	if status := postJSON(t, srv.URL+"/v1/query", req, &a); status != http.StatusCreated {
		t.Fatalf("POST /v1/query = %d", status)
	}
	if a.Answer.Len() != 1 || a.Answer.Tuples[0][1].Text() != "114" || !a.Written || a.Receipt.Program == nil || a.Hash != a.Receipt.ServedHash {
		t.Errorf("answer = %+v", a)
	}
	req.DryRun = true
	if status := postJSON(t, srv.URL+"/v1/query", req, &a); status != http.StatusOK || a.Written {
		t.Errorf("dry run = %d, written %v", status, a.Written)
	}

	var e api.Error
	bad := req
	bad.Text = `high(E) :- nothing(E).`
	if status := postJSON(t, srv.URL+"/v1/query", bad, &e); status != http.StatusBadRequest || !strings.Contains(e.Error, "unknown relation nothing/1") {
		t.Errorf("bad program = %d %q", status, e.Error)
	}
	resp, err := http.Post(srv.URL+"/v1/query", "application/json", strings.NewReader(`{"projection": "bp", "nope": 1}`)) // #nosec G107 -- loopback
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("unknown field = %d", resp.StatusCode)
	}
}

// Over max_read_bytes the derivation is refused with its measure and the query's
// narrowings, before anything is filed.
func TestQueryRoutes_BoundsTheDerivation(t *testing.T) {
	cfg := newConfig(t)
	cfg.MaxReadBytes = 64
	seedReadings(t, cfg)
	d := open(t, cfg)
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	req := api.QueryRequest{Projection: "bp", Relation: "leaf", Text: `leaf(E, P, V) :- attr(E, P, V).`}
	var e api.Error
	if status := postJSON(t, srv.URL+"/v1/query", req, &e); status != http.StatusRequestEntityTooLarge {
		t.Fatalf("POST /v1/query over the bound = %d %q", status, e.Error)
	}
	if e.Code != api.CodeTooLarge || e.TooLarge == nil || e.TooLarge.MaxBytes != 64 || !strings.Contains(strings.Join(e.TooLarge.Narrow, " "), "instant") {
		t.Errorf("refusal = %+v", e)
	}
	var receipts api.Receipts
	getJSON(t, srv.URL+"/v1/receipts", http.StatusOK, &receipts)
	if len(receipts.Receipts) != 0 {
		t.Errorf("a refused query filed %d receipts", len(receipts.Receipts))
	}
}
