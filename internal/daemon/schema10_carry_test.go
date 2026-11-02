package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

// Card 260: a store the schema-9 daemon wrote (opstest.Schema9) is carried into a fresh
// schema-10 store by the functions anam move-artifacts --registered-from calls, and a
// schema-10 daemon on it answers what the schema-9 daemon answered. The reads at the
// same watermarks match, every receipt is reproduced and intact with the same notes,
// and the selection codes and holds are the same.

// schema9Expect is the fixture's expect.json, what the schema-9 daemon answered.
type schema9Expect struct {
	W          int64                          `json:"w"`
	LedgerID   string                         `json:"ledger_id"`
	Reads      map[string]string              `json:"reads"`
	Receipts   map[string]schema9Receipt      `json:"receipts"`
	Selections []api.Selection                `json:"selections"`
	Holds      []string                       `json:"holds"`
	Verified   map[string]api.ReceiptVerified `json:"verified"`
}

type schema9Receipt struct {
	How          string `json:"how"`
	Template     string `json:"template"`
	Version      int    `json:"template_version"`
	TemplateHash string `json:"template_hash"`
}

// schema9Reads are the reads expect.json recorded, by name, as timeline queries.
var schema9Reads = map[string]string{
	"timeline":               "zone=UTC&w=%d",
	"timeline weights":       "zone=UTC&w=%d&projection=weights",
	"timeline regweights":    "zone=UTC&w=%d&projection=regweights",
	"timeline earlier":       "zone=UTC&w=3",
	"timeline type narrowed": "zone=UTC&w=%d&type=health.weight",
}

// carriedSchema9 writes the fixture, carries its store into a fresh schema-10 store and
// returns a config over the fixture's ledger and the new store, on the catalog the
// schema-9 daemon ran on.
func carriedSchema9(t *testing.T) (config.Config, opstest.Schema9Paths, schema9Expect) {
	t.Helper()
	ctx := context.Background()
	p, err := opstest.Schema9(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := opstest.Schema9File("expect.json")
	if err != nil {
		t.Fatal(err)
	}
	var exp schema9Expect
	if err := json.Unmarshal(raw, &exp); err != nil {
		t.Fatal(err)
	}

	cfg := rolesConfig(t)
	cfg.LedgerPath = p.Ledger
	cfg.DBPath = filepath.Join(t.TempDir(), "tracking.db")
	cfg.DefinitionsDir, cfg.ProjectionsDir = p.Definitions, p.Projections

	src, err := ops.ReadCarrySource(ctx, p.Store)
	if err != nil {
		t.Fatal(err)
	}
	buckets, err := span.New(span.Default)
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CarrySchema(ctx, src.Registered); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return cfg, p, exp
}

func TestSchema10_ACarriedSchema9StoreAnswersAsTheSchema9DaemonDid(t *testing.T) {
	t.Parallel()
	cfg, _, exp := carriedSchema9(t)
	d, url, stop := serveImporters(t, cfg)
	if got := head(t, d); got != exp.W {
		t.Fatalf("the fixture ledger is at W=%d, expect.json says %d", got, exp.W)
	}

	for name, q := range schema9Reads {
		if strings.Contains(q, "%d") {
			q = fmt.Sprintf(q, exp.W)
		}
		var r api.Timeline
		call(t, "GET", url+"/v1/timeline?"+q, testToken, "", http.StatusOK, &r)
		if got := fmt.Sprintf("w=%d canonical=%s live=%d shown=%d", r.W, r.Canonical, r.Live, r.Shown); got != exp.Reads[name] {
			t.Errorf("%s: schema 10 reads %s, schema 9 read %s", name, got, exp.Reads[name])
		}
	}

	for hash, want := range exp.Verified {
		var got api.ReceiptVerified
		call(t, "GET", url+"/v1/receipts/"+hash+"/verify", testToken, "", http.StatusOK, &got)
		if !got.Result.OK() || !reflect.DeepEqual(got, want) {
			t.Errorf("%s (%s): schema 10 verifies it as\n%+v\nschema 9 as\n%+v", hash, exp.Receipts[hash].How, got, want)
		}
	}

	for _, want := range exp.Selections {
		var got api.Selection
		call(t, "GET", url+"/v1/selections/sel:"+want.Code, testToken, "", http.StatusOK, &got)
		if got != want {
			t.Errorf("sel:%s opens as %+v, was saved as %+v", want.Code, got, want)
		}
	}

	var status api.Status
	call(t, "GET", url+"/v1/status", testToken, "", http.StatusOK, &status)
	if !slices.Equal(status.Held, exp.Holds) {
		t.Errorf("held %v after the carry, %v before it", status.Held, exp.Holds)
	}
	for _, path := range []string{"/v1/projections/regweights/1", "/v1/projections/regweights/2",
		"/v1/definitions/test.reading/1", "/v1/definitions/test.reading/2"} {
		call(t, "GET", url+path, testToken, "", http.StatusOK, nil)
	}

	// A receipt filed after the carry, through the catalog template the old receipts
	// were served through, names the same template hash, files no second body, and
	// verifies after a restart.
	var pub api.Published
	call(t, "POST", url+"/v1/receipts?zone=UTC&projection=weights", testToken, "", http.StatusCreated, &pub)
	var oldWeights string
	for _, r := range exp.Receipts {
		if r.Template == "weights" {
			oldWeights = r.TemplateHash
		}
	}
	if pub.Receipt.TemplateHash != oldWeights {
		t.Errorf("a new weights receipt names template %s, the carried ones %s", pub.Receipt.TemplateHash, oldWeights)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	_, url, _ = serveImporters(t, cfg)
	var ver api.ReceiptVerified
	call(t, "GET", url+"/v1/receipts/"+pub.ReceiptHash+"/verify", testToken, "", http.StatusOK, &ver)
	if !ver.Result.OK() || ver.Receipt.TemplateHash != oldWeights {
		t.Errorf("the new receipt after a restart: %s, template %s", ver.Result.Summary(), ver.Receipt.TemplateHash)
	}
	db, err := ops.OpenReadOnly(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if bodies, err := db.List(context.Background(), ops.KindTemplateBody); err != nil || len(bodies) != len(uniqueBodies(exp)) {
		t.Errorf("template bodies after a new receipt: %v (%v), want the %d carried", bodies, err, len(uniqueBodies(exp)))
	}
}

func uniqueBodies(exp schema9Expect) map[string]bool {
	out := map[string]bool{}
	for _, r := range exp.Receipts {
		out[r.TemplateHash] = true
	}
	return out
}

// The carry is what lets a receipt outlive its template file. On a catalog that has
// moved weights to version 2 and dropped test.gone, every weights v1 receipt the
// schema-9 daemon filed verifies from the body carried into template_body, and the
// provisioned history of the dropped type is still in the store after the daemon has
// started and stopped on it.
func TestSchema10_ACarriedBodyReplaysAReceiptWhoseTemplateLeftTheCatalog(t *testing.T) {
	t.Parallel()
	cfg, p, exp := carriedSchema9(t)
	defs, projs := filepath.Join(t.TempDir(), "definitions"), filepath.Join(t.TempDir(), "projections")
	if err := os.CopyFS(defs, os.DirFS(p.Definitions)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(defs, "test.gone.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projs, 0o750); err != nil {
		t.Fatal(err)
	}
	v2 := `{"name": "weights", "version": 2, "description": "weigh-ins, from the catalog, second version",
  "types": ["health.weight"], "statuses": ["admitted", "endorsed"], "max_classification": "Sensitive",
  "from": "", "to": "", "shape": "log"}`
	if err := os.WriteFile(filepath.Join(projs, "weights.json"), []byte(v2), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.DefinitionsDir, cfg.ProjectionsDir = defs, projs

	_, url, stop := serveImporters(t, cfg)
	seen := 0
	for hash, r := range exp.Receipts {
		if r.Template != "weights" {
			continue
		}
		seen++
		var v api.ReceiptVerified
		call(t, "GET", url+"/v1/receipts/"+hash+"/verify", testToken, "", http.StatusOK, &v)
		if !v.Result.OK() || !slices.Contains(v.Result.Notes,
			"replayed weights v1 from the body filed with the receipt; the catalog no longer holds that version") {
			t.Errorf("%s (%s) on a catalog without weights v1: %s", hash, r.How, v.Result.Summary())
		}
	}
	if seen != 3 {
		t.Fatalf("the fixture has %d weights receipts, want 3", seen)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}

	db, err := ops.OpenReadOnly(context.Background(), cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Definitions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(rows, func(r ops.DefinitionRow) bool { return r.Type == "test.gone" && r.Version == 1 }) {
		t.Errorf("the provisioned test.gone v1 is gone after a start on a catalog without it: %d rows", len(rows))
	}
}
