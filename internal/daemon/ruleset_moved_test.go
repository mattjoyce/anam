package daemon

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/receipt"
)

// A receipt that names a rule set no text held here has is a moved rule set, and
// verify answers it 409 as it does a moved template, not 500 (card 182, card 212).
// A receipt under the bundled text still verifies, 200.
func TestVerify_AMovedRuleSetIs409(t *testing.T) {
	cfg := rolesConfig(t)
	d, url, _ := serveImporters(t, cfg)

	weight, _ := d.defs.current.Load().Lookup("health.weight")
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.5, weight.Version)), http.StatusOK, nil)

	var a api.QueryAnswer
	call(t, "POST", url+"/v1/query", readerToken, inlineQuery(inlineWeightsBody, ""), http.StatusCreated, &a)
	var ok api.ReceiptVerified
	call(t, "GET", url+"/v1/receipts/"+a.ReceiptHash+"/verify", readerToken, "", http.StatusOK, &ok)
	if !ok.Result.OK() {
		t.Fatalf("the receipt under the bundled rules did not verify: %s", ok.Result.Summary())
	}

	// The same receipt, naming a rule set this binary does not hold.
	moved := a.Receipt
	prog := *moved.Program
	prog.RuleSetHash = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	moved.Program = &prog
	raw, err := moved.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	hash, err := d.writes.Ops.Put(context.Background(), receipt.Kind, raw)
	if err != nil {
		t.Fatalf("filing the forged receipt: %v", err)
	}

	var e api.Error
	call(t, "GET", url+"/v1/receipts/"+hash+"/verify", readerToken, "", http.StatusConflict, &e)
	if !strings.Contains(e.Error, "rule set has moved") {
		t.Errorf("the 409 does not say the rule set has moved: %q", e.Error)
	}
}
