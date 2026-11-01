package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// An index derived from another ledger, or claiming events this one does not hold,
// would be served as an answer. The daemon does not start on one.
func TestOpenRefusesAWrongIndex(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		spoil func(t *testing.T, db *ops.DB)
		want  error
	}{
		{"derived from another ledger", func(t *testing.T, db *ops.DB) {
			if err := db.PinLedger(context.Background(), "another-ledger"); err != nil {
				t.Fatal(err)
			}
		}, ops.ErrLedgerChanged},
		{"ahead of the ledger", func(t *testing.T, db *ops.DB) {
			if err := db.ApplyReplay(context.Background(), 0, 7, nil, nil, nil); err != nil {
				t.Fatal(err)
			}
		}, catchup.ErrIndexAhead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newConfig(t)
			buckets, _ := cfg.Buckets()
			db, err := ops.Open(context.Background(), cfg.DBPath, buckets)
			if err != nil {
				t.Fatal(err)
			}
			tt.spoil(t, db)
			_ = db.Close()
			if _, err := Open(context.Background(), cfg, Options{}); !errors.Is(err, tt.want) {
				t.Fatalf("Open: got %v, want %v", err, tt.want)
			}
		})
	}
}

// Once a catch-up has found the index wrong, the routes that read it refuse rather
// than answer from it.
func TestAWrongIndexIsRefusedNotServed(t *testing.T) {
	t.Parallel()
	d := open(t, newConfig(t))
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	d.index.set(catchup.ErrIndexAhead)
	for _, path := range []string{"/v1/changes?since=2000-01-01&zone=UTC", "/v1/checkpoints"} {
		var e map[string]string
		getJSON(t, srv.URL+path, http.StatusInternalServerError, &e)
		if !strings.Contains(e["error"], "ahead") {
			t.Errorf("GET %s: %q", path, e["error"])
		}
	}
}

// A payload reaches the client as it is held: HTML in it is not escaped, so the CLI
// renders it as it would from local files.
func TestWriteJSONKeepsPayloadBytes(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	writeJSON(rec, http.StatusOK, fold.Entry{Payload: json.RawMessage(`{"note":"<a&b>"}`)})
	if body := rec.Body.String(); !strings.Contains(body, `"<a&b>"`) {
		t.Errorf("body %s escaped the payload", body)
	}
}

// The daemon works to the shorter of its timeout and the caller's, so it answers
// before the caller gives up.
func TestTimeoutIsTheShorterOfTheTwo(t *testing.T) {
	t.Parallel()
	d := &Daemon{}
	d.cfg.Timeout = 5 * time.Second
	for _, tt := range []struct {
		header string
		want   time.Duration
	}{
		{"", 5 * time.Second},
		{"2s", 2 * time.Second},
		{"30s", 5 * time.Second},
		{"nonsense", 5 * time.Second},
		{"-1s", 5 * time.Second},
	} {
		r := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
		r.Header.Set(TimeoutHeader, tt.header)
		if got := d.timeout(r); got != tt.want {
			t.Errorf("timeout with %q = %s, want %s", tt.header, got, tt.want)
		}
	}
}
