package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/client"
)

// The daemon is stood in for here: these are the answers the client must turn into
// errors a person can act on, which the daemon's own tests cannot provoke.
func TestClientErrors(t *testing.T) {
	t.Parallel()
	tokenPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenPath, []byte(strings.Repeat("t", 32)), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tt := range []struct {
		name    string
		token   string
		handler http.HandlerFunc
		want    string
	}{
		{"no token configured", "", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}, "wants a token"},
		{"a token refused", tokenPath, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
		}, "refused the token"},
		{"an older daemon", tokenPath, func(w http.ResponseWriter, _ *http.Request) {
			http.NotFound(w, nil)
		}, "older build"},
		{"an error body", tokenPath, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"--from: bad"}`))
		}, "--from: bad (400 Bad Request)"},
		{"the caller's deadline is sent", tokenPath, func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte(`{"error":"` + r.Header.Get("Anam-Timeout") + `"}`))
		}, "3s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(tt.handler)
			defer srv.Close()
			c, err := client.New(srv.URL, tt.token, 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = c.Status(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), srv.URL) {
				t.Fatalf("Status: %v, want an error naming the server and saying %q", err, tt.want)
			}
		})
	}
}
