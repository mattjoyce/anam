package daemon

import (
	"net/http"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// Card 211: a reader asks the record a one-off question by saying in the request which
// types it reads through, with no projection registered first. The receipt verifies
// after the daemon restarts, because the template's body is filed by its hash.

const inlineWeightsBody = `{"types": ["health.weight"], "statuses": ["admitted", "endorsed"], "max_classification": "Sensitive"}`

const weightQuery = `high(E, V) :- entry(E, "health.weight", _, _, _, _, _), attr(E, "/kg", V), V > 80.`

func inlineQuery(projection string, extra string) string {
	return `{"projection": ` + projection + `, "relation": "high", "text": "` + strings.ReplaceAll(weightQuery, `"`, `\"`) + `"` + extra + `}`
}

func TestInlineProjection_AReaderQueriesAndTheReceiptVerifiesAfterARestart(t *testing.T) {
	t.Parallel()
	cfg := rolesConfig(t)
	d, url, stop := serveImporters(t, cfg)

	weight, _ := d.defs.current.Load().Lookup("health.weight")
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.5, weight.Version), weighIn("w-2", 79.0, weight.Version)), http.StatusOK, nil)

	var a api.QueryAnswer
	call(t, "POST", url+"/v1/query", readerToken, inlineQuery(inlineWeightsBody, ""), http.StatusCreated, &a)
	r := a.Receipt
	if a.Answer.Len() != 1 || a.Answer.Tuples()[0][1].Text() != "80.5" {
		t.Fatalf("answer = %+v", a.Answer)
	}
	if !a.Written || !r.TemplateInline || r.Template != "inline" || r.TemplateVersion != 1 || !strings.HasPrefix(r.TemplateHash, "sha256:") {
		t.Fatalf("receipt = %+v", r)
	}
	if a.Projection.Name != "inline" || a.Projection.Served != 2 {
		t.Errorf("projection read = %+v", a.Projection)
	}

	// The same template laid out another way is the same template; another set of types
	// is another.
	var same api.QueryAnswer
	call(t, "POST", url+"/v1/query", readerToken,
		inlineQuery(`{"max_classification":"Sensitive","statuses":["admitted","endorsed"],"types":["health.weight"]}`, `, "dry_run": true`),
		http.StatusOK, &same)
	if same.Receipt.TemplateHash != r.TemplateHash || same.Written {
		t.Errorf("same template: hash %s written %v, want %s and a dry run", same.Receipt.TemplateHash, same.Written, r.TemplateHash)
	}

	var ver api.ReceiptVerified
	call(t, "GET", url+"/v1/receipts/"+a.ReceiptHash+"/verify", readerToken, "", http.StatusOK, &ver)
	if !ver.Result.OK() {
		t.Fatalf("verify before the restart: %s", ver.Result.Summary())
	}

	if err := stop(); err != nil {
		t.Fatal(err)
	}
	_, url, _ = serveImporters(t, cfg)
	call(t, "GET", url+"/v1/receipts/"+a.ReceiptHash+"/verify", readerToken, "", http.StatusOK, &ver)
	if !ver.Result.OK() || ver.Receipt.TemplateHash != r.TemplateHash {
		t.Fatalf("verify after the restart: %s", ver.Result.Summary())
	}
	var shown api.ReceiptShown
	call(t, "GET", url+"/v1/receipts/"+a.ReceiptHash, readerToken, "", http.StatusOK, &shown)
}

// A malformed inline template is refused as the reader's error, naming the field it got
// wrong, before anything is read: no answer, and no receipt or body filed.
func TestInlineProjection_AMalformedTemplateIsRefusedNamingTheField(t *testing.T) {
	t.Parallel()
	cfg := rolesConfig(t)
	_, url, _ := serveImporters(t, cfg)

	for name, tc := range map[string]struct{ projection, field string }{
		"no types":         {`{"statuses": [], "max_classification": "Normal"}`, "types"},
		"a bad type":       {`{"types": ["Not A Type"], "statuses": [], "max_classification": "Normal"}`, "types"},
		"a bad status":     {`{"types": [], "statuses": ["maybe"], "max_classification": "Normal"}`, "statuses"},
		"a bad ceiling":    {`{"types": [], "statuses": [], "max_classification": "Secret"}`, "max_classification"},
		"a bad shape":      {`{"types": [], "statuses": [], "max_classification": "Normal", "shape": "pile"}`, "shape"},
		"a bad from":       {`{"types": [], "statuses": [], "max_classification": "Normal", "from": "yesterday"}`, "from"},
		"a name":           {`{"name": "x", "types": [], "statuses": [], "max_classification": "Normal"}`, "name"},
		"an unknown key":   {`{"types": [], "statuses": [], "max_classification": "Normal", "bogus": 1}`, "bogus"},
		"no Definition":    {`{"types": ["health.nothing"], "statuses": [], "max_classification": "Normal"}`, "health.nothing"},
		"a number":         {`7`, "projection"},
		"an array":         {`["health.weight"]`, "projection"},
		"an empty object":  {`{}`, "types"},
		"neither name nor": {`""`, "projection"},
	} {
		var e api.Error
		call(t, "POST", url+"/v1/query", readerToken, inlineQuery(tc.projection, ""), http.StatusBadRequest, &e)
		if !strings.Contains(e.Error, tc.field) {
			t.Errorf("%s: %q does not name %q", name, e.Error, tc.field)
		}
	}

	var receipts api.Receipts
	call(t, "GET", url+"/v1/receipts", readerToken, "", http.StatusOK, &receipts)
	if len(receipts.Receipts) != 0 {
		t.Errorf("refused queries filed %d receipts", len(receipts.Receipts))
	}
}
