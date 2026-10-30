package inspect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 106: a library document is untrusted material the person inspects, not a page
// of the inspector. The API and the inspector share an origin, so an HTML or SVG file
// served inline with no sandbox would run with the operator's authority and could
// read the record. Every document is served sandboxed; active kinds are downloads.
func TestDocument_ActiveContentCannotRunAsTheInspector(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	library := t.TempDir()
	files := map[string]string{
		"evil.html":  `<script>fetch('/v1/status')</script>`,
		"evil.svg":   `<svg xmlns="http://www.w3.org/2000/svg"><script>fetch('/v1/status')</script></svg>`,
		"evil.xhtml": `<html xmlns="http://www.w3.org/1999/xhtml"><script>1</script></html>`,
		"note.txt":   "plain words",
		"scan.pdf":   "%PDF-1.4",
		"photo.png":  "\x89PNG",
	}
	sums := map[string]string{}
	for name, body := range files {
		sums[name] = store(t, library, filepath.Ext(name), body)
	}
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = l.Close() })
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Catalog: testCatalog(), Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Library: library}))
	t.Cleanup(srv.Close)

	for name := range files {
		resp, err := http.Get(srv.URL + "/document?sha256=" + sums[name] + "&name=" + url.QueryEscape(name)) //nolint:gosec // test URL
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("%s: %d", name, resp.StatusCode)
			continue
		}
		csp := resp.Header.Get("Content-Security-Policy")
		if !strings.Contains(csp, "sandbox") || strings.Contains(csp, "allow-scripts") || strings.Contains(csp, "allow-same-origin") {
			t.Errorf("%s: Content-Security-Policy = %q, want a sandbox that allows neither scripts nor the origin", name, csp)
		}
		if got := resp.Header.Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
			t.Errorf("%s: Cross-Origin-Resource-Policy = %q", name, got)
		}
		cd := resp.Header.Get("Content-Disposition")
		active := strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".svg") || strings.HasSuffix(name, ".xhtml")
		switch {
		case active && !strings.HasPrefix(cd, "attachment"):
			t.Errorf("%s is active content served %q, want an attachment", name, cd)
		case !active && !strings.HasPrefix(cd, "inline"):
			t.Errorf("%s is a safe preview served %q, want inline", name, cd)
		}
	}
}
