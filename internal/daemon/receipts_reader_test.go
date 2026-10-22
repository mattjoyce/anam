package daemon

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
)

const (
	recorderToken = "recorder-token-0123456789abcdef01"
	plainToken    = "plain-token-0123456789abcdef0123"
)

// readerConfig is a daemon with the blood-pressure readings, an operator, a reader
// named assistant whose every read is filed as a receipt, and a plain reader.
func readerConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := newConfig(t)
	seedReadings(t, cfg)
	dir := t.TempDir()
	cfg.TokenFile = writeToken(t, dir, testToken)
	hash := func(tok string) string { s := sha256.Sum256([]byte(tok)); return hex.EncodeToString(s[:]) }
	body := fmt.Sprintf(`principals:
  - name: assistant
    role: reader
    token_sha256: %s
    receipts: always
  - name: plain
    role: reader
    token_sha256: %s
`, hash(recorderToken), hash(plainToken))
	cfg.ImportersFile = filepath.Join(dir, "importers.yaml")
	if err := os.WriteFile(cfg.ImportersFile, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func listReceipts(t *testing.T, base, query string) api.Receipts {
	t.Helper()
	var l api.Receipts
	call(t, "GET", base+"/v1/receipts"+query, testToken, "", http.StatusOK, &l)
	return l
}

const timelineBP = "/v1/timeline?zone=UTC&projection=bp"

// A token set to record leaves one receipt per timeline read, naming its principal,
// and a token without the setting leaves none.
func TestRecordingTokenFilesAReceiptPerTimelineRead(t *testing.T) {
	d := open(t, readerConfig(t))
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	base := srv.URL

	var plain api.Timeline
	call(t, "GET", base+timelineBP, plainToken, "", http.StatusOK, &plain)
	if n := len(listReceipts(t, base, "").Receipts); n != 0 {
		t.Fatalf("a plain reader's timeline read left %d receipts", n)
	}

	var got api.Published
	call(t, "GET", base+timelineBP, recorderToken, "", http.StatusOK, &got)
	if len(got.Entries) != 2 || got.Shown != 2 || got.Page.NextCursor != "" {
		t.Errorf("a recording reader's timeline carries %d entries, shown %d, page %+v; want the two readings whole",
			len(got.Entries), got.Shown, got.Page)
	}
	if got.W != plain.W || got.Projection == nil || plain.Projection == nil || got.Projection.Hash != plain.Projection.Hash {
		t.Errorf("the recorded read is not the plain one: W %d vs %d", got.W, plain.W)
	}
	if got.ReceiptHash == "" || got.Receipt.Principal != "assistant" || got.Receipt.ServedHash != got.Projection.Hash {
		t.Errorf("receipt = %+v", got.Receipt)
	}

	call(t, "GET", base+timelineBP, recorderToken, "", http.StatusOK, &got)
	call(t, "GET", base+timelineBP, plainToken, "", http.StatusOK, &plain)
	listed := listReceipts(t, base, "")
	if len(listed.Receipts) != 2 {
		t.Fatalf("two recorded reads left %d receipts", len(listed.Receipts))
	}
	for _, l := range listed.Receipts {
		if l.Receipt.Principal != "assistant" {
			t.Errorf("receipt %s names %q", l.Hash, l.Receipt.Principal)
		}
	}
}

// A read a receipt cannot describe is refused to a recording token, not served
// unrecorded.
func TestRecordingTokenRefusesAReadNoReceiptCanDescribe(t *testing.T) {
	d := open(t, readerConfig(t))
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	var e api.Error
	call(t, "GET", srv.URL+"/v1/timeline?zone=UTC", recorderToken, "", http.StatusBadRequest, &e)
	if e.Error == "" {
		t.Error("the refusal says nothing")
	}
	call(t, "GET", srv.URL+timelineBP+"&explain=true", recorderToken, "", http.StatusBadRequest, nil)
	// The same reads stay open to a token that does not record.
	call(t, "GET", srv.URL+"/v1/timeline?zone=UTC", plainToken, "", http.StatusOK, nil)
	if n := len(listReceipts(t, srv.URL, "").Receipts); n != 0 {
		t.Errorf("refused reads left %d receipts", n)
	}
}

// A query by a recording token is filed naming it; a dry run stays a dry run; a query
// by any other token names nobody.
func TestRecordingTokenFilesAReceiptPerQuery(t *testing.T) {
	d := open(t, readerConfig(t))
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	base := srv.URL

	req := `{"projection": "bp", "relation": "high", "text": "high(T, S) :- entry(E, \"health.blood-pressure\", T, _, _, _, _), attr(E, \"/systolic_blood_pressure/value\", S), S > 110."}`
	var a api.QueryAnswer
	call(t, "POST", base+"/v1/query", recorderToken, req, http.StatusCreated, &a)
	if !a.Written || a.Receipt.Principal != "assistant" {
		t.Errorf("recorded query: written %v, principal %q", a.Written, a.Receipt.Principal)
	}

	dry := `{"projection": "bp", "relation": "high", "dry_run": true, "text": "high(T, S) :- entry(E, \"health.blood-pressure\", T, _, _, _, _), attr(E, \"/systolic_blood_pressure/value\", S), S > 110."}`
	call(t, "POST", base+"/v1/query", recorderToken, dry, http.StatusOK, &a)
	if a.Written {
		t.Error("a dry run was filed")
	}

	var other api.QueryAnswer
	call(t, "POST", base+"/v1/query", plainToken, req, http.StatusCreated, &other)
	if other.Receipt.Principal != "" {
		t.Errorf("a plain reader's query names %q", other.Receipt.Principal)
	}
	if n := len(listReceipts(t, base, "").Receipts); n != 2 {
		t.Errorf("%d receipts, want the recorded query's and the plain one's", n)
	}
}

// Who read what: the listing filters by principal and by served time, and a receipt
// of either kind verifies.
func TestReceiptsListByPrincipalAndTime(t *testing.T) {
	d := open(t, readerConfig(t))
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	base := srv.URL

	var rec, pub api.Published
	call(t, "GET", base+timelineBP, recorderToken, "", http.StatusOK, &rec)
	call(t, "POST", base+"/v1/receipts?zone=UTC&projection=bp", plainToken, "", http.StatusCreated, &pub)
	if pub.Receipt.Principal != "" {
		t.Fatalf("an ordinary publish names %q", pub.Receipt.Principal)
	}

	if n := len(listReceipts(t, base, "").Receipts); n != 2 {
		t.Fatalf("%d receipts listed, want 2", n)
	}
	by := listReceipts(t, base, "?principal=assistant")
	if len(by.Receipts) != 1 || by.Receipts[0].Hash != rec.ReceiptHash {
		t.Errorf("principal=assistant listed %+v", by.Receipts)
	}
	if n := len(listReceipts(t, base, "?principal=nobody").Receipts); n != 0 {
		t.Errorf("principal=nobody listed %d", n)
	}
	served := rec.Receipt.ServedAt
	for _, c := range []struct {
		query string
		want  int
	}{
		{"?since=2000-01-01&until=2999-01-01&zone=UTC", 2},
		{"?since=2999-01-01&zone=UTC", 0},
		{"?until=2000-01-01&zone=UTC", 0},
		{"?since=" + url.QueryEscape(served) + "&principal=assistant", 1},
		{"?until=" + url.QueryEscape(served) + "&principal=assistant", 0},
	} {
		if n := len(listReceipts(t, base, c.query).Receipts); n != c.want {
			t.Errorf("%s listed %d, want %d", c.query, n, c.want)
		}
	}
	call(t, "GET", base+"/v1/receipts?since=yesterday&zone=UTC", testToken, "", http.StatusBadRequest, nil)
	call(t, "GET", base+"/v1/receipts?since=2026-01-01", testToken, "", http.StatusBadRequest, nil)

	for _, h := range []string{rec.ReceiptHash, pub.ReceiptHash} {
		var v api.ReceiptVerified
		call(t, "GET", base+"/v1/receipts/"+h+"/verify", testToken, "", http.StatusOK, &v)
		if !v.Result.OK() {
			t.Errorf("receipt %s does not verify: %s", h, v.Result.Summary())
		}
	}
}

// A recording token opens only the receipts that name it, since opening another's
// would be a read no receipt of its own records. Other tokens open any.
func TestRecordingTokenOpensOnlyItsOwnReceipts(t *testing.T) {
	d := open(t, readerConfig(t))
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()
	base := srv.URL

	var mine, theirs api.Published
	call(t, "GET", base+timelineBP, recorderToken, "", http.StatusOK, &mine)
	call(t, "POST", base+"/v1/receipts?zone=UTC&projection=bp", plainToken, "", http.StatusCreated, &theirs)

	var shown api.ReceiptShown
	call(t, "GET", base+"/v1/receipts/"+mine.ReceiptHash, recorderToken, "", http.StatusOK, &shown)
	if len(shown.Entries) != 2 {
		t.Errorf("its own receipt shows %d entries", len(shown.Entries))
	}
	id := mine.Entries[0].FacetID
	call(t, "GET", base+"/v1/receipts/"+mine.ReceiptHash+"/facets/"+url.PathEscape(id), recorderToken, "", http.StatusOK, nil)

	call(t, "GET", base+"/v1/receipts/"+theirs.ReceiptHash, recorderToken, "", http.StatusForbidden, nil)
	call(t, "GET", base+"/v1/receipts/"+theirs.ReceiptHash+"/facets/"+url.PathEscape(id), recorderToken, "", http.StatusForbidden, nil)
	call(t, "GET", base+"/v1/receipts/sha256:"+fmt.Sprintf("%064x", 1), recorderToken, "", http.StatusNotFound, nil)

	for _, tok := range []string{plainToken, testToken} {
		call(t, "GET", base+"/v1/receipts/"+mine.ReceiptHash, tok, "", http.StatusOK, nil)
		call(t, "GET", base+"/v1/receipts/"+theirs.ReceiptHash, tok, "", http.StatusOK, nil)
	}
}

// When the receipt cannot be filed, the read fails and the reader gets no facets: a
// recorded read is the receipt and the answer together.
func TestRecordingTokenReadFailsWhenTheReceiptCannotBeFiled(t *testing.T) {
	d := open(t, readerConfig(t))
	t.Cleanup(func() { _ = d.closeStores() })
	srv := httptest.NewServer(d.Handler())
	defer srv.Close()

	if err := d.writes.Ops.Close(); err != nil {
		t.Fatal(err)
	}
	var e api.Error
	call(t, "GET", srv.URL+timelineBP, recorderToken, "", http.StatusInternalServerError, &e)
	if e.Error == "" {
		t.Error("the failure says nothing")
	}
	// A plain read does not write, so it is unaffected.
	call(t, "GET", srv.URL+timelineBP, plainToken, "", http.StatusOK, nil)
}
