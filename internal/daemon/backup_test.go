package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
)

// A backup asked of the daemon lands in backup_dir as a whole record, taken on the
// write handles the daemon holds; without a backup_dir it is refused, not guessed.
func TestBackupOverHTTP(t *testing.T) {
	t.Parallel()
	post := func(t *testing.T, url string) (*http.Response, api.Backup) {
		t.Helper()
		resp, err := http.Post(url+"/v1/backups", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = resp.Body.Close() }()
		var b api.Backup
		_ = json.NewDecoder(resp.Body).Decode(&b)
		return resp, b
	}

	cfg := newConfig(t)
	cfg.BackupDir, cfg.BackupKeep = filepath.Join(t.TempDir(), "backups"), 3
	d := open(t, cfg)
	start(t, d)
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	resp, b := post(t, srv.URL)
	if resp.StatusCode != http.StatusOK || filepath.Dir(b.Path) != cfg.BackupDir || b.Files != 2 {
		t.Fatalf("POST /v1/backups: %d %+v", resp.StatusCode, b)
	}
	for _, f := range []string{"ledger.db", "tracking.db"} {
		if _, err := os.Stat(filepath.Join(b.Path, f)); err != nil {
			t.Errorf("backup lacks %s: %v", f, err)
		}
	}

	none := open(t, newConfig(t))
	start(t, none)
	srv2 := httptest.NewServer(none.Handler())
	defer srv2.Close()
	if resp, _ := post(t, srv2.URL); resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("POST /v1/backups without a backup_dir: %d, want 503", resp.StatusCode)
	}
}
