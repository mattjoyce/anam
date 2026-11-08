package inspect_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catalog/catalogtest"
	"github.com/mattjoyce/anam/internal/inspect"
)

// The feed is served as files under /feed/: the page, and the script it loads.
func TestFeedIsServed(t *testing.T) {
	t.Parallel()
	l := emptyLedger(t)
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Catalog: catalogtest.Engine.Catalog(t), Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct{ path, ctype, holds string }{
		{"/feed/", "text/html", `<script src="fixture.js">`},
		{"/feed/index.html", "text/html", "Anam"},
		{"/feed/fixture.js", "text/javascript", "window.ANAM_FIXTURE"},
	} {
		res, err := http.Get(srv.URL + tc.path)
		if err != nil {
			t.Fatalf("GET %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(res.Body)
		_ = res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: status %d", tc.path, res.StatusCode)
		}
		if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, tc.ctype) {
			t.Errorf("GET %s: content type %q, want %s", tc.path, ct, tc.ctype)
		}
		if !strings.Contains(string(body), tc.holds) {
			t.Errorf("GET %s: body lacks %q", tc.path, tc.holds)
		}
	}

	// A bare /feed lands on the directory, so the page's relative script path resolves.
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Get(srv.URL + "/feed")
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusMovedPermanently || res.Header.Get("Location") != "/feed/" {
		t.Errorf("GET /feed: %d to %q, want 301 to /feed/", res.StatusCode, res.Header.Get("Location"))
	}
}
