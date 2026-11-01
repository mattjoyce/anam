package daemon

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Card 108: a page on another site must not be able to make the person's browser
// start operator work, whether the daemon is tokenless on loopback or the browser has
// cached the token for basic auth. Only the browser's own fetch metadata can say where
// a request came from, so that is what is checked, before any work.
func TestBrowserGuard_RefusesForeignOriginsOnUnsafeRoutes(t *testing.T) {
	t.Parallel()
	cfg := newConfig(t)
	cfg.TokenFile = writeToken(t, t.TempDir(), testToken)
	d := open(t, cfg)
	t.Cleanup(func() { _ = d.closeStores() })
	h := d.Handler()

	tests := []struct {
		name, method, path, host string
		headers                  map[string]string
		want403                  bool
	}{
		{"the CLI: no browser headers", "POST", "/v1/checkpoints?dry_run=true", "anam.local:8787", nil, false},
		{"the inspector's own form", "POST", "/view", "anam.local:8787",
			map[string]string{"Origin": "http://anam.local:8787", "Sec-Fetch-Site": "same-origin"}, false},
		{"a typed address", "POST", "/view", "anam.local:8787", map[string]string{"Sec-Fetch-Site": "none"}, false},
		{"another site, by Sec-Fetch-Site", "POST", "/v1/checkpoints", "anam.local:8787",
			map[string]string{"Origin": "https://untrusted.example", "Sec-Fetch-Site": "cross-site"}, true},
		{"another site, by Origin alone", "POST", "/v1/backups", "anam.local:8787",
			map[string]string{"Origin": "https://untrusted.example"}, true},
		{"a sandboxed page", "POST", "/v1/checkpoints", "anam.local:8787", map[string]string{"Origin": "null"}, true},
		{"a same-site subdomain is still another origin", "POST", "/view", "anam.local:8787",
			map[string]string{"Origin": "http://evil.anam.local:8787", "Sec-Fetch-Site": "same-site"}, true},
		{"a read from another site is not the guard's business", "GET", "/v1/status", "anam.local:8787",
			map[string]string{"Origin": "https://untrusted.example", "Sec-Fetch-Site": "cross-site"}, false},
		{"an importer's post carries no browser headers", "POST", "/v1/envelopes", "anam.local:8787", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader("x=y"))
			req.Host = tt.host
			req.Header.Set("Authorization", "Bearer "+testToken)
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if got := rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "another site"); got != tt.want403 {
				t.Errorf("%s %s: %d %s; want refused=%v", tt.method, tt.path, rec.Code, rec.Body.String(), tt.want403)
			}
		})
	}
}

// Without a token the daemon is on loopback and every request is ambient-authorised,
// so the Host must be a loopback name too: a DNS name pointed at this machine is how a
// page elsewhere would reach it.
func TestBrowserGuard_TokenlessNeedsALoopbackHost(t *testing.T) {
	t.Parallel()
	cfg := newConfig(t)
	d := open(t, cfg)
	t.Cleanup(func() { _ = d.closeStores() })
	h := d.Handler()
	for host, want403 := range map[string]bool{
		"127.0.0.1:8787": false, "localhost:8787": false, "[::1]:8787": false,
		"rebound.example:8787": true, "192.168.20.4:8787": true,
	} {
		req := httptest.NewRequest("POST", "/v1/checkpoints?dry_run=true", nil)
		req.Host = host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if got := rec.Code == http.StatusForbidden; got != want403 {
			t.Errorf("host %s: %d %s; want refused=%v", host, rec.Code, rec.Body.String(), want403)
		}
		get := httptest.NewRequest("GET", "/v1/status", nil)
		get.Host = host
		rec = httptest.NewRecorder()
		h.ServeHTTP(rec, get)
		if rec.Code == http.StatusForbidden {
			t.Errorf("host %s: a read was refused", host)
		}
	}
}
