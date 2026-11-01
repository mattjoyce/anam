package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/token"
)

const testToken = "0123456789abcdef0123456789abcdef"

func writeToken(t *testing.T, dir, token string) string {
	t.Helper()
	p := filepath.Join(dir, "token")
	if err := os.WriteFile(p, []byte(token+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTokenFile(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		token  string
		listen string
		want   error
	}{
		{"a token lets the daemon leave loopback", testToken, "0.0.0.0:8787", nil},
		{"a short token is refused", "too-short", "127.0.0.1:0", token.ErrWeak},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(t)
			cfg.TokenFile = writeToken(t, t.TempDir(), tt.token)
			cfg.Listen = tt.listen
			d, err := Open(context.Background(), cfg, Options{})
			if !errors.Is(err, tt.want) {
				t.Fatalf("Open: got %v, want %v", err, tt.want)
			}
			if d != nil {
				_ = d.closeStores()
			}
		})
	}
}

func TestEverythingButHealthzNeedsTheToken(t *testing.T) {
	t.Parallel()
	cfg := newConfig(t)
	cfg.TokenFile = writeToken(t, t.TempDir(), testToken)
	d := open(t, cfg)
	t.Cleanup(func() { _ = d.closeStores() })
	h := d.Handler()

	tests := []struct {
		name, path string
		auth       func(*http.Request)
		want       int
	}{
		{"healthz needs nothing", "/healthz", func(*http.Request) {}, http.StatusOK},
		{"status without a token", "/v1/status", func(*http.Request) {}, http.StatusUnauthorized},
		{"inspector without a token", "/", func(*http.Request) {}, http.StatusUnauthorized},
		{"a document without a token", "/document?sha256=x", func(*http.Request) {}, http.StatusUnauthorized},
		{"the wrong bearer token", "/v1/status",
			func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testToken[1:]+"x") }, http.StatusUnauthorized},
		{"the right bearer token", "/v1/status",
			func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+testToken) }, http.StatusOK},
		{"the token as a basic-auth password", "/",
			func(r *http.Request) { r.SetBasicAuth("matt", testToken); r.URL.RawQuery = "view=whole" }, http.StatusOK},
		{"the token as a basic-auth user name", "/",
			func(r *http.Request) { r.SetBasicAuth(testToken, "") }, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			tt.auth(req)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("GET %s: status %d, want %d", tt.path, rec.Code, tt.want)
			}
		})
	}
}
