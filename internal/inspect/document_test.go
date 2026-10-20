package inspect_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/mattjoyce/anam/internal/library"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// store puts body in the library as Anam does and returns its sha256.
func store(t *testing.T, dir, ext, body string) string {
	t.Helper()
	s := sha256.Sum256([]byte(body))
	sum := hex.EncodeToString(s[:])
	if _, err := library.Store(dir, 1000, sum, ext, []byte(body)); err != nil {
		t.Fatal(err)
	}
	return sum
}

// Cards 29 and 150: a reference facet's page links to the document by its sha256, the
// inspector serves the bytes from the default library as they are, under the name the
// facet gave them, and nothing outside the library root can be reached.
func TestDocument_ServedFromTheLibraryAndConfinedToIt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	dir := t.TempDir()
	sum := store(t, dir, ".pdf", "%PDF-1.4 the bytes")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("not yours"), 0o600); err != nil {
		t.Fatal(err)
	}
	linked := strings.Repeat("a", 64)
	if err := os.Symlink(outside, filepath.Join(dir, linked+".txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	ref := seedFacet(t, facet.SourceRef("documents", "scans/report.pdf"), "record.document",
		`{"path": "scans/report.pdf", "sha256": "`+sum+`", "media_type": "application/pdf", "bytes": 18, "date": "2026-01-30"}`)
	ref.Descriptor.PayloadKind = facet.PayloadReference
	if err := l.Admit(ctx, 0, ref, event(facet.EventAdmitted, ref, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Library: dir}))
	t.Cleanup(srv.Close)

	link := "/document?name=report.pdf&amp;sha256=" + sum
	code, body := get(t, srv.URL+"/facet?id="+ref.ID)
	if code != http.StatusOK || !strings.Contains(body, `href="`+link+`"`) {
		t.Fatalf("facet page %d does not link the document as %s:\n%s", code, link, body)
	}

	resp, err := http.Get(srv.URL + "/document?sha256=" + sum + "&name=report.pdf") //nolint:gosec // test URL
	if err != nil {
		t.Fatalf("GET document: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != "application/pdf" {
		t.Fatalf("document = %d %s, want 200 application/pdf", resp.StatusCode, resp.Header.Get("Content-Type"))
	}
	if cd := resp.Header.Get("Content-Disposition"); cd != `inline; filename=report.pdf` {
		t.Errorf("Content-Disposition = %q", cd)
	}

	for _, bad := range []string{linked, strings.Repeat("0", 64), "../secret.txt", sum[:63], strings.ToUpper(sum), ""} {
		code, _ := get(t, srv.URL+"/document?sha256="+url.QueryEscape(bad))
		if code != http.StatusNotFound {
			t.Errorf("sha256 %q = %d, want 404", bad, code)
		}
	}

	// No library configured: the page carries no link and the route serves nothing.
	bare := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(bare.Close)
	if _, body := get(t, bare.URL+"/facet?id="+ref.ID); strings.Contains(body, "/document?") {
		t.Errorf("facet page links a document with no library configured")
	}
	if code, _ := get(t, bare.URL+"/document?sha256="+sum); code != http.StatusNotFound {
		t.Errorf("document route with no library = %d, want 404", code)
	}
}
