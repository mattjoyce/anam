package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatus_ReadsADaemonThatAnswersPersonOnly is card 270a's slice 2 against the
// daemon that runs before it: v0.794-ed2a028 answers person and no subject. A newer
// CLI asking it still names the subject, from person.
func TestStatus_ReadsADaemonThatAnswersPersonOnly(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/status" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"version":"v0.794-ed2a028","person":"matt","ledger_id":"0d1e2f3a","w":81948,"since":"2026-11-04T00:00:00.000000000Z","sources":[]}`)
	}))
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	token := filepath.Join(dir, "token")
	if err := os.WriteFile(token, []byte(testToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(cfg, []byte(fmt.Sprintf("server: %s\ntoken_file: %s\n", srv.URL, token)), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _, err := cli(t, cfg, "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ledger  0d1e2f3a (subject matt)\n") {
		t.Errorf("status against a person-only daemon:\n%s", out)
	}
}
