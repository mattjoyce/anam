package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
)

// Cards 102 and 150: an importer hands Anam a document's bytes, Anam alone writes the
// library and names each file by its sha256, and a reference to bytes the library does
// not hold is refused at the door.

const docToken = "documents-token-0123456789abcdef0123456789"

func documentsConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := newConfig(t)
	dir := t.TempDir()
	cfg.TokenFile = writeToken(t, dir, testToken)
	cfg.ImportersFile = filepath.Join(dir, "importers.yaml")
	docSum := sha256.Sum256([]byte(docToken))
	otherSum := sha256.Sum256([]byte(importerToken))
	body := fmt.Sprintf("importers:\n"+
		"  - source: documents\n    token_sha256: %s\n    types: [record.document]\n"+
		"  - source: test-import\n    token_sha256: %s\n    types: [health.weight]\n",
		hex.EncodeToString(docSum[:]), hex.EncodeToString(otherSum[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func reference(version int, path, sum string, size int) string {
	return fmt.Sprintf(`{"type": "record.document", "definition_version": %d, "payload": {"path": %q, "sha256": %q, "media_type": "application/pdf", "bytes": %d, "date": "2026-01-30", "source_time": {"value": "2026-01-30", "zone": "Australia/Sydney", "basis": "assumption"}}}`,
		version, path, sum, size)
}

func TestPostDocument(t *testing.T) {
	t.Parallel()
	cfg := documentsConfig(t)
	d, base, _ := serveImporters(t, cfg)
	ref, _ := d.defs.current.Load().Lookup(docType)
	post := func(token, ext, body string, want int) api.StoredDocument {
		t.Helper()
		var out api.StoredDocument
		into := any(&out)
		if want >= 300 {
			into = nil
		}
		call(t, "POST", base+"/v1/documents?ext="+url.QueryEscape(ext), token, body, want, into)
		return out
	}
	hexOf := func(b string) string { s := sha256.Sum256([]byte(b)); return hex.EncodeToString(s[:]) }
	letter, rescan := "%PDF-1.4 a scanned letter", "%PDF-1.4 the letter, scanned again"

	got := post(docToken, "pdf", letter, http.StatusCreated)
	if !got.New || got.SHA256 != hexOf(letter) || got.Bytes != int64(len(letter)) || got.MediaType != "application/pdf" {
		t.Fatalf("first post: %+v", got)
	}
	held, err := os.ReadFile(filepath.Join(cfg.LibraryDir, hexOf(letter)+".pdf"))
	if err != nil || string(held) != letter {
		t.Fatalf("the library holds %q, %v", held, err)
	}
	if again := post(docToken, "PDF", letter, http.StatusOK); again.New {
		t.Errorf("the same bytes again were new: %+v", again)
	}
	if other := post(docToken, "pdf", rescan, http.StatusCreated); !other.New {
		t.Errorf("other bytes were not new: %+v", other)
	}
	for _, bad := range []string{"../x", "p/df", "pdf.meta", "a b", strings.Repeat("x", 17)} {
		post(docToken, bad, letter, http.StatusBadRequest)
	}
	post(importerToken, "pdf", letter, http.StatusForbidden) // may not submit record.document
	call(t, "POST", base+"/v1/documents?ext=pdf", testToken, letter, http.StatusForbidden, nil)
	if entries, _ := os.ReadDir(cfg.LibraryDir); len(entries) != 2 {
		t.Errorf("the library holds %d entries, want the two documents (no temporaries)", len(entries))
	}

	// A reference to stored bytes is admitted; the same name with the rescan's bytes
	// supersedes it, and the first bytes stay. A reference to bytes the library does
	// not hold is refused whole, with nothing written.
	name := "scans/2026-01-30_letter.pdf"
	var a api.Admission
	call(t, "POST", base+"/v1/envelopes", docToken, batch(reference(ref.Version, name, hexOf(letter), len(letter))), http.StatusOK, &a)
	if a.Changed != 1 {
		t.Fatalf("admission of a held document: %+v", a)
	}
	call(t, "POST", base+"/v1/envelopes", docToken, batch(reference(ref.Version, name, hexOf(rescan), len(rescan))), http.StatusOK, &a)
	if a.Changed != 1 {
		t.Fatalf("a rescan under the same name: %+v", a)
	}
	if _, err := os.Stat(filepath.Join(cfg.LibraryDir, hexOf(letter)+".pdf")); err != nil {
		t.Errorf("the first scan's bytes are gone: %v", err)
	}
	var refused api.Refusal
	call(t, "POST", base+"/v1/envelopes", docToken, batch(
		reference(ref.Version, "scans/never-posted.pdf", hexOf("never posted"), 12),
		reference(ref.Version, name, strings.Repeat("0", 64), len(letter)),
	), http.StatusUnprocessableEntity, &refused)
	if len(refused.Refused) != 2 {
		t.Fatalf("refusal: %+v", refused)
	}
	for _, e := range refused.Refused {
		if !strings.Contains(e.Reason, "does not hold") {
			t.Errorf("refusal reason %q does not say the library lacks the document", e.Reason)
		}
	}
	var st api.Status
	call(t, "GET", base+"/v1/status", testToken, "", http.StatusOK, &st)
	if st.W != 2 {
		t.Errorf("W = %d after a refused batch, want 2", st.W)
	}
}
