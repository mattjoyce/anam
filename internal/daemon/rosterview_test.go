package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
)

const (
	testVersion = "v0.564-3ba0f29"
	testCommit  = "3ba0f29c0ffee0123456789abcdef0123456789a"
	testBuilt   = "2026-10-23T09:00:00Z"
)

// serveBuilt runs a daemon on cfg that reports a build, and returns its URL.
func serveBuilt(t *testing.T, cfg config.Config) (*Daemon, string) {
	t.Helper()
	d, err := Open(context.Background(), cfg, Options{Version: testVersion, GitCommit: testCommit, BuildDate: testBuilt})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	start(t, d)
	srv := httptest.NewServer(d.Handler())
	t.Cleanup(srv.Close)
	return d, srv.URL
}

// withReader declares a reader, assistant, in cfg's importers file beside what is there.
func withReader(t *testing.T, cfg config.Config) {
	t.Helper()
	sum := sha256.Sum256([]byte(readerToken))
	f, err := os.OpenFile(cfg.ImportersFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := fmt.Fprintf(f, "principals:\n  - name: assistant\n    role: reader\n    token_sha256: %s\n    receipts: always\n",
		hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
}

// operatorPage fetches a Workbench page as the operator and returns its body.
func operatorPage(t *testing.T, base, path string) string {
	t.Helper()
	req, err := http.NewRequest("GET", base+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+testToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: %d: %s", path, resp.StatusCode, b)
	}
	return string(b)
}

// Card 228: every Workbench page carries the build the daemon reports, the string
// GET /v1/status gives, with the commit and the build date on hover.
func TestWorkbenchFrameShowsTheDaemonsVersion(t *testing.T) {
	t.Parallel()
	cfg := holdConfig(t)
	_, base := serveBuilt(t, cfg)

	var st api.Status
	call(t, "GET", base+"/v1/status", testToken, "", http.StatusOK, &st)
	if st.Version != testVersion {
		t.Fatalf("status version %q, want %q", st.Version, testVersion)
	}
	want := fmt.Sprintf(`id="build" title="commit %s, built %s">%s</span>`, testCommit, testBuilt, st.Version)
	for _, path := range []string{"/", "/facets", "/events", "/definitions", "/projection", "/select", "/location", "/importers", "/entry?type=health.medication.dose"} {
		if body := operatorPage(t, base, path); !strings.Contains(body, want) {
			t.Errorf("GET %s: the frame does not show the daemon's version; want %s", path, want)
		}
	}
}

// Card 228: the importers page shows each principal's rules from the policy package, the
// source's last batch and hold, and a token only as the first eight characters of its
// stored hash.
func TestImportersPageShowsThePolicyAndRedactsTheToken(t *testing.T) {
	t.Parallel()
	cfg := holdConfig(t)
	withReader(t, cfg)
	d, base := serveBuilt(t, cfg)

	sums := map[string]string{}
	for name, tok := range map[string]string{"importer": importerToken, "second": secondToken, "reader": readerToken, "operator": testToken} {
		sum := sha256.Sum256([]byte(tok))
		sums[name] = hex.EncodeToString(sum[:])
	}
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	call(t, "POST", base+"/v1/envelopes", importerToken, batch(weighIn("w-1", 80, weight.Version)), http.StatusOK, nil)
	call(t, "POST", base+"/v1/hold", testToken, `{"sources": ["second-import"]}`, http.StatusOK, nil)

	body := operatorPage(t, base, "/importers")
	for name, sum := range sums {
		if !strings.Contains(body, sum[:8]) {
			t.Errorf("%s: the page does not show the token's eight-character prefix %s", name, sum[:8])
		}
		if strings.Contains(body, sum[:9]) {
			t.Errorf("%s: the page shows more than eight characters of the stored hash", name)
		}
	}
	for _, secret := range []string{importerToken, secondToken, readerToken, testToken} {
		if strings.Contains(body, secret) {
			t.Errorf("the page shows a token: %q", secret)
		}
	}
	for _, want := range []string{
		"test-import", "second-import", "assistant", "workbench", "no token",
		"health.weight", "test.reading", "records every read", "lists receipts naming assistant",
		"opens only receipts naming it", "snapshots of test-import", "never held",
		"POST /v1/envelopes", "held<", "may be held; not held now", "admitted",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the page lacks %q:\n%s", want, body)
		}
	}
}

// Card 228: the importers page is the operator's: a reader's or an importer's token is
// refused, as for every other Workbench page, and a request with no token is not known.
func TestImportersPageIsTheOperatorsAlone(t *testing.T) {
	t.Parallel()
	cfg := holdConfig(t)
	withReader(t, cfg)
	_, base := serveBuilt(t, cfg)

	for name, tt := range map[string]struct {
		token string
		want  int
	}{
		"reader": {readerToken, http.StatusForbidden}, "importer": {importerToken, http.StatusForbidden},
		"nobody": {"", http.StatusUnauthorized},
	} {
		t.Run(name, func(t *testing.T) {
			status, body, _ := holdCall(t, "GET", base+"/importers", tt.token, "")
			if status != tt.want {
				t.Fatalf("GET /importers as %s: %d, want %d: %s", name, status, tt.want, body)
			}
			if strings.Contains(body, "token") && strings.Contains(body, "<table") {
				t.Errorf("a refused request got the page: %s", body)
			}
		})
	}
}
