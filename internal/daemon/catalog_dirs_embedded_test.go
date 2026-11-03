package daemon

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/projection"
)

// A receipt minted on the built-in catalog verifies the same on a directory copied
// from it: same result, same notes, no "definition set has moved". Where the files came
// from never enters a content hash. This is the live transition (R2b) an installation
// makes when it moves from the embedded catalog to configured directories, so it starts
// on the embedded catalog on purpose. Allow-listed for R4a (card 255); R4b removes it
// with the embedded catalog. TestReceiptVerifiesTheSameOnACopiedCatalog is the same
// check between two fixture directories.
func TestReceiptMintedOnTheEmbeddedCatalogVerifiesOnACopy(t *testing.T) {
	t.Parallel()
	cfg := readerConfig(t)
	cfg.DefinitionsDir, cfg.ProjectionsDir = "", "" // the built-in catalog

	d := open(t, cfg)
	srv := httptest.NewServer(d.Handler())
	var minted api.Published
	call(t, "GET", srv.URL+timelineBP, recorderToken, "", http.StatusOK, &minted)
	var embedded api.ReceiptVerified
	call(t, "GET", srv.URL+"/v1/receipts/"+minted.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &embedded)
	srv.Close()
	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}
	if !embedded.Result.OK() {
		t.Fatalf("the receipt does not verify on the built-in catalog: %s", embedded.Result.Summary())
	}
	if src := d.catalog.DefinitionsSource(); src != "embedded" {
		t.Fatalf("the first start read definitions from %q, want the embedded catalog", src)
	}

	cfg.DefinitionsDir = copyCatalogRoot(t, def.EmbeddedFS())
	cfg.ProjectionsDir = copyCatalogRoot(t, projection.EmbeddedFS())
	d2 := open(t, cfg)
	t.Cleanup(func() { _ = d2.closeStores() })
	srv2 := httptest.NewServer(d2.Handler())
	defer srv2.Close()
	var copied api.ReceiptVerified
	call(t, "GET", srv2.URL+"/v1/receipts/"+minted.ReceiptHash+"/verify", recorderToken, "", http.StatusOK, &copied)

	if copied.Result.OK() != embedded.Result.OK() || copied.Result.Summary() != embedded.Result.Summary() {
		t.Errorf("verification differs on a copied catalog:\n built-in: %s\n copied:   %s",
			embedded.Result.Summary(), copied.Result.Summary())
	}
	if !reflect.DeepEqual(copied.Result, embedded.Result) {
		t.Errorf("verification result differs on a copied catalog:\n built-in: %+v\n copied:   %+v", embedded.Result, copied.Result)
	}
}
