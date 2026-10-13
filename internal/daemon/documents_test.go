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

// Card 102: an importer hands Anam a document's bytes, Anam alone writes the library,
// and a reference to bytes the library does not hold is refused at the door.

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
		"  - source: documents\n    token_sha256: %s\n    types: [doc.reference]\n"+
		"  - source: test-import\n    token_sha256: %s\n    types: [health.weight]\n",
		hex.EncodeToString(docSum[:]), hex.EncodeToString(otherSum[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func reference(version int, path, sum string, size int) string {
	return fmt.Sprintf(`{"type": "doc.reference", "definition_version": %d, "payload": {"path": %q, "sha256": %q, "media_type": "application/pdf", "bytes": %d, "date": "2026-01-30"}}`,
		version, path, sum, size)
}

func TestPostDocument(t *testing.T) {
	cfg := documentsConfig(t)
	d, base, _ := serveImporters(t, cfg)
	ref, _ := d.defs.current.Load().Lookup(docType)
	post := func(token, path, body string, want int) api.StoredDocument {
		t.Helper()
		var out api.StoredDocument
		into := any(&out)
		if want >= 300 {
			into = nil
		}
		call(t, "POST", base+"/v1/documents?path="+url.QueryEscape(path), token, body, want, into)
		return out
	}
	letter := "%PDF-1.4 a scanned letter"
	sum := sha256.Sum256([]byte(letter))
	hexSum := hex.EncodeToString(sum[:])

	got := post(docToken, "scans/2026-01-30_letter.pdf", letter, http.StatusCreated)
	if !got.New || got.SHA256 != hexSum || got.Bytes != int64(len(letter)) || got.MediaType != "application/pdf" {
		t.Fatalf("first post: %+v", got)
	}
	held, err := os.ReadFile(filepath.Join(cfg.LibraryDir, "scans", "2026-01-30_letter.pdf"))
	if err != nil || string(held) != letter {
		t.Fatalf("the library holds %q, %v", held, err)
	}
	if again := post(docToken, "scans/2026-01-30_letter.pdf", letter, http.StatusOK); again.New {
		t.Errorf("the same bytes again were new: %+v", again)
	}
	post(docToken, "scans/2026-01-30_letter.pdf", letter+" changed", http.StatusConflict) // a value, never overwritten
	for _, bad := range []string{"../out.pdf", "/etc/passwd", "a/../b.pdf", ".hidden.pdf", "a/.git/x", "x.pdf.meta.yaml", "", "a//b.pdf"} {
		post(docToken, bad, letter, http.StatusBadRequest)
	}
	post(importerToken, "w.pdf", letter, http.StatusForbidden) // may not submit doc.reference
	call(t, "POST", base+"/v1/documents?path=x.pdf", testToken, letter, http.StatusForbidden, nil)
	if entries, _ := os.ReadDir(filepath.Join(cfg.LibraryDir, "scans")); len(entries) != 1 {
		t.Errorf("the library's scans folder holds %d entries, want the one document (no temporaries)", len(entries))
	}

	// A reference to the stored bytes is admitted; to a path or bytes the library
	// does not hold, refused whole, with nothing written.
	var a api.Admission
	call(t, "POST", base+"/v1/envelopes", docToken, batch(reference(ref.Version, "scans/2026-01-30_letter.pdf", hexSum, len(letter))), http.StatusOK, &a)
	if a.Changed != 1 {
		t.Fatalf("admission of a held document: %+v", a)
	}
	var refused api.Refusal
	call(t, "POST", base+"/v1/envelopes", docToken, batch(
		reference(ref.Version, "scans/never-posted.pdf", hexSum, len(letter)),
		reference(ref.Version, "scans/2026-01-30_letter.pdf", strings.Repeat("0", 64), len(letter)),
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
	if st.W != 1 {
		t.Errorf("W = %d after a refused batch, want 1", st.W)
	}
}
