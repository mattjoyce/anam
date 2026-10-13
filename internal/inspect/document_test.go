package inspect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 29: a reference facet's page links to the document, the inspector serves the
// bytes from the default library as they are, and nothing outside the library root
// can be reached by any spelling of the path.
func TestDocument_ServedFromTheLibraryAndConfinedToIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	library := t.TempDir()
	if err := os.MkdirAll(filepath.Join(library, "scans"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(library, "scans", "report.pdf"), []byte("%PDF-1.4 the bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("not yours"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(library, "link.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	ref := seedFacet(t, facet.SourceRef("documents", "scans/report.pdf"), "doc.reference",
		`{"path": "scans/report.pdf", "sha256": "00", "media_type": "application/pdf", "bytes": 18, "date": "2026-01-30"}`)
	ref.Descriptor.PayloadKind = facet.PayloadReference
	if err := l.Admit(ctx, 0, ref, event(facet.EventAdmitted, ref, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Library: library}))
	t.Cleanup(srv.Close)

	code, body := get(t, srv.URL+"/facet?id="+ref.ID)
	if code != http.StatusOK || !strings.Contains(body, `href="/document?path=scans%2Freport.pdf"`) {
		t.Fatalf("facet page %d does not link the document:\n%s", code, body)
	}

	resp, err := http.Get(srv.URL + "/document?path=" + url.QueryEscape("scans/report.pdf")) //nolint:gosec // test URL
	if err != nil {
		t.Fatalf("GET document: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("document = %d %s, want 200 application/pdf", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, "inline") || !strings.Contains(cd, "report.pdf") {
		t.Errorf("Content-Disposition = %q", cd)
	}

	for _, bad := range []string{"../secret.txt", "scans/../../secret.txt", "link.txt", "/etc/passwd", "", "scans"} {
		code, _ := get(t, srv.URL+"/document?path="+url.QueryEscape(bad))
		if code != http.StatusNotFound {
			t.Errorf("path %q = %d, want 404", bad, code)
		}
	}

	// No library configured: the page carries no link and the route serves nothing.
	bare := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(bare.Close)
	if _, body := get(t, bare.URL+"/facet?id="+ref.ID); strings.Contains(body, "/document?path=") {
		t.Errorf("facet page links a document with no library configured")
	}
	if code, _ := get(t, bare.URL+"/document?path=scans%2Freport.pdf"); code != http.StatusNotFound {
		t.Errorf("document route with no library = %d, want 404", code)
	}
}
