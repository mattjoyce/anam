package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/catchup"
)

// Card 91 with card 104: one facet as a receipt served it, beside the links the
// record holds to it. A note about a weigh-in is an edge touching it; the receipt
// served the weigh-in, so the facet opens with the note as a link on its to side; the
// note itself was not served, so it does not open through this receipt; and a facet
// nobody served is 404 however real it is.
func TestGetReceiptFacet_EntryWithItsLinks(t *testing.T) {
	t.Parallel()
	cfg := newConfig(t)
	dir := t.TempDir()
	cfg.TokenFile = writeToken(t, dir, testToken)
	sum := sha256.Sum256([]byte(importerToken))
	cfg.ImportersFile = filepath.Join(dir, "importers.yaml")
	body := fmt.Sprintf("importers:\n  - source: test-import\n    token_sha256: %s\n    types: [health.weight, record.edge]\n", hex.EncodeToString(sum[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d, url, _ := serveImporters(t, cfg)
	call(t, "POST", url+"/v1/projections", testToken, weightsV1, http.StatusCreated, nil)

	weight, _ := d.defs.current.Load().Lookup("health.weight")
	note, _ := d.defs.current.Load().Lookup("record.edge")
	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.5, weight.Version), weighIn("w-2", 80.1, weight.Version)), http.StatusOK, &a)
	w1, w2 := a.Results[0].FacetID, a.Results[1].FacetID
	noteEnv := fmt.Sprintf(`{"type": "record.edge", "definition_version": %d,
		"payload": {"id": "note:1", "at": "2026-10-09 08:00", "to": [%q], "text": "Fasted, after the run.", "source_time": {"value": "2026-10-09 08:00", "zone": "Australia/Sydney", "offset": "+11:00", "basis": "source"}}}`, note.Version, w1)
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(noteEnv), http.StatusOK, &a)
	noteID := a.Results[0].FacetID

	var pub api.Published
	call(t, "POST", url+"/v1/receipts?zone=UTC&projection=weights", testToken, "", http.StatusCreated, &pub)
	if len(pub.Receipt.FacetIDs) != 2 {
		t.Fatalf("published %d facets through weights, want 2", len(pub.Receipt.FacetIDs))
	}

	var shown api.FacetShown
	call(t, "GET", url+"/v1/receipts/"+pub.ReceiptHash+"/facets/"+w1, testToken, "", http.StatusOK, &shown)
	if shown.Hash != pub.ReceiptHash || shown.Entry.FacetID != w1 || shown.Entry.Type != "health.weight" || shown.Entry.ContentHash == "" {
		t.Fatalf("shown = %+v", shown)
	}
	if shown.Links.W != head(t, d) || len(shown.Links.Edges) != 1 {
		t.Fatalf("links = %+v, want one edge at W %d", shown.Links, head(t, d))
	}
	if l := shown.Links.Edges[0]; l.Edge != noteID || l.Kind != "about" || l.Side != "to" || len(l.From) != 0 || len(l.To) != 1 || l.To[0] != w1 || l.At == "" {
		t.Errorf("link = %+v", l)
	}

	// The other weigh-in was served and nothing names it: links present, empty.
	call(t, "GET", url+"/v1/receipts/"+pub.ReceiptHash+"/facets/"+w2, testToken, "", http.StatusOK, &shown)
	if shown.Entry.FacetID != w2 || shown.Links.Edges == nil || len(shown.Links.Edges) != 0 {
		t.Errorf("w2 shown = %+v", shown)
	}

	// Not served by this receipt: the note, and a facet that does not exist.
	call(t, "GET", url+"/v1/receipts/"+pub.ReceiptHash+"/facets/"+noteID, testToken, "", http.StatusNotFound, nil)
	call(t, "GET", url+"/v1/receipts/"+pub.ReceiptHash+"/facets/no-such-facet", testToken, "", http.StatusNotFound, nil)
	call(t, "GET", url+"/v1/receipts/sha256:"+hex.EncodeToString(sum[:])+"/facets/"+w1, testToken, "", http.StatusNotFound, nil)
}

// The inspector reads the same operational index, behind the same index state. Once
// a catch-up has found the index wrong, the facet page refuses as the API's facet
// route does, rather than draw the facet with links the index got wrong; the timeline,
// which is the ledger's, stays up and draws the checkpoint strip red with the words.
func TestInspector_SaysWhenTheIndexIsWrong(t *testing.T) {
	t.Parallel()
	cfg := newConfig(t)
	dir := t.TempDir()
	cfg.TokenFile = writeToken(t, dir, testToken)
	sum := sha256.Sum256([]byte(importerToken))
	cfg.ImportersFile = filepath.Join(dir, "importers.yaml")
	body := fmt.Sprintf("importers:\n  - source: test-import\n    token_sha256: %s\n    types: [health.weight]\n", hex.EncodeToString(sum[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d, url, _ := serveImporters(t, cfg)
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	var a api.Admission
	call(t, "POST", url+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80.5, weight.Version)), http.StatusOK, &a)
	w1 := a.Results[0].FacetID

	call(t, "GET", url+"/facet?id="+w1, testToken, "", http.StatusOK, nil)
	if _, body := page(t, http.MethodGet, url+"/?view=whole", nil, nil); strings.Contains(body, "index is wrong") || !strings.Contains(body, `class="bucket`) {
		t.Errorf("before: the timeline should draw the strip:\n%s", body)
	}
	d.index.set(catchup.ErrIndexAhead)
	call(t, "GET", url+"/facet?id="+w1, testToken, "", http.StatusInternalServerError, nil)
	code, body := page(t, http.MethodGet, url+"/?view=whole", nil, nil)
	if code != http.StatusOK {
		t.Fatalf("after: the timeline answered %d, want 200", code)
	}
	for _, want := range []string{`<g class="wrong">`, "checkpoint strip not drawn: inspect: the operational index is wrong: catchup: replay cursor is ahead", "anam timeline"} {
		if !strings.Contains(body, want) {
			t.Errorf("after: the timeline lacks %q", want)
		}
	}
	if strings.Contains(body, `class="bucket `) {
		t.Error("after: the strip still draws buckets from a wrong index")
	}
}
