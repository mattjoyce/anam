package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
)

const readerToken = "reader-token-0123456789abcdef0123"

// rolesConfig is importerConfig with a reader, assistant, beside the importer.
func rolesConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := importerConfig(t)
	sum := sha256.Sum256([]byte(readerToken))
	extra := fmt.Sprintf("principals:\n  - name: assistant\n    role: reader\n    token_sha256: %s\n", hex.EncodeToString(sum[:]))
	f, err := os.OpenFile(cfg.ImportersFile, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(extra); err != nil {
		t.Fatal(err)
	}
	return cfg
}

var wildcard = regexp.MustCompile(`\{[^}]+\}`)

// Card 89: every route admits exactly the roles its entry in the route table names. A
// token of each role is sent to every route; a role the route names is let through to
// the handler (whatever it answers to an empty request), and any other is 403.
func TestEveryRouteAdmitsTheRolesItNames(t *testing.T) {
	cfg := rolesConfig(t)
	d := open(t, cfg)
	stop := start(t, d)
	t.Cleanup(func() { _ = stop() })
	h := d.Handler()

	tokens := map[config.Role]string{
		config.RoleReader: readerToken, config.RoleImporter: importerToken, config.RoleOperator: testToken,
	}
	for _, rt := range d.routes() {
		method, path, ok := strings.Cut(rt.pattern, " ")
		if !ok {
			method, path = http.MethodGet, rt.pattern
		}
		path = wildcard.ReplaceAllString(path, "x")
		for role, token := range tokens {
			t.Run(fmt.Sprintf("%s as %s", rt.pattern, role), func(t *testing.T) {
				req := httptest.NewRequest(method, path, strings.NewReader("{}"))
				req.Header.Set("Authorization", "Bearer "+token)
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				refused := rec.Code == http.StatusUnauthorized ||
					rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "holds role")
				if want := slices.Contains(rt.roles, role); refused == want {
					t.Fatalf("%s %s as %s: %d %s; want admitted %v", method, path, role, rec.Code, rec.Body.String(), want)
				}
			})
		}
	}
}

// Card 89: what a reader may not do, said apart from the route table, so a change to
// the table that opened one of these to readers fails here.
func TestAReaderWritesNothingButReceipts(t *testing.T) {
	_, url, _ := serveImporters(t, rolesConfig(t))
	for _, tt := range []struct{ method, path string }{
		{"POST", "/v1/envelopes"},
		{"POST", "/v1/definitions"},
		{"POST", "/v1/documents"},
		{"POST", "/v1/checkpoints"},
		{"POST", "/v1/backups"},
		{"POST", "/v1/projections"},
		{"GET", "/"},
	} {
		call(t, tt.method, url+tt.path, readerToken, "{}", http.StatusForbidden, nil)
	}
	call(t, "GET", url+"/v1/status", readerToken, "", http.StatusOK, nil)
	call(t, "GET", url+"/v1/definitions", readerToken, "", http.StatusOK, nil)
	call(t, "POST", url+"/v1/receipts?zone=UTC&projection=meds", readerToken, "", http.StatusCreated, nil)
}

// Card 89: a principal in the importers file may not hold the operator's token.
func TestAReaderMayNotHoldTheOperatorsToken(t *testing.T) {
	cfg := importerConfig(t)
	sum := sha256.Sum256([]byte(testToken))
	body := fmt.Sprintf("principals:\n  - name: assistant\n    role: reader\n    token_sha256: %s\n", hex.EncodeToString(sum[:]))
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	d, err := Open(t.Context(), cfg, Options{})
	if d != nil {
		_ = d.closeStores()
	}
	if err == nil || !strings.Contains(err.Error(), "operator's token") {
		t.Fatalf("Open: got %v, want the operator's token refused", err)
	}
}
