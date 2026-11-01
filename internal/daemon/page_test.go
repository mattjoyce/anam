package daemon

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"testing"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/fold"
)

// Card 92: a read too large for one answer is published once and paged. Walking
// every page from the publish's next cursor gives back exactly the receipt's
// facet_ids, in order, each page within max_read_bytes; the receipt rides the first
// page only; anything but a cursor this receipt issued is refused 400
// cursor_invalid; new facets admitted between pages change no page; and the receipt
// still verifies.
func TestReceiptPages_WalkTheWholePublishedRead(t *testing.T) {
	t.Parallel()
	cfg := importerConfig(t)
	cfg.MaxReadBytes = 1200
	d, base, _ := serveImporters(t, cfg)
	call(t, "POST", base+"/v1/projections", testToken, weightsV1, http.StatusCreated, nil)
	weight, _ := d.defs.current.Load().Lookup("health.weight")
	var envs []string
	for i := range 8 {
		envs = append(envs, weighIn(fmt.Sprintf("w-%d", i), 80+float64(i)/10, weight.Version))
	}
	call(t, "POST", base+"/v1/envelopes", importerToken, batch(envs...), http.StatusOK, nil)

	var pub api.Published
	call(t, "POST", base+"/v1/receipts?zone=UTC&projection=weights", testToken, "", http.StatusCreated, &pub)
	if len(pub.Receipt.FacetIDs) != 8 || pub.Shown != 8 || pub.Page.TotalCount != 8 {
		t.Fatalf("published %d facets, shown %d, page %+v; want 8", len(pub.Receipt.FacetIDs), pub.Shown, pub.Page)
	}
	if pub.Page.NextCursor == "" || pub.Page.ReturnedCount != len(pub.Entries) || len(pub.Entries) >= 8 {
		t.Fatalf("the first page = %+v with %d entries; want a cut page and a next cursor", pub.Page, len(pub.Entries))
	}
	if len(pub.Versions) != len(pub.Entries) {
		t.Errorf("versions name %d facets, the page carries %d", len(pub.Versions), len(pub.Entries))
	}

	// Anam learns something between pages; no page is a new read, so none shows it.
	call(t, "POST", base+"/v1/envelopes", importerToken, batch(weighIn("w-late", 79.9, weight.Version)), http.StatusOK, nil)

	ids := idsOf(pub.Entries)
	cursors := []string{}
	pages := 1
	for next := pub.Page.NextCursor; next != ""; pages++ {
		var page api.ReceiptShown
		call(t, "GET", base+"/v1/receipts/"+pub.ReceiptHash+"?cursor="+url.QueryEscape(next), testToken, "", http.StatusOK, &page)
		if page.Receipt != nil || page.Hash != pub.ReceiptHash || page.Page == nil {
			t.Fatalf("page %d: receipt %v, hash %s, page %+v; want the hash and the page, no receipt", pages+1, page.Receipt != nil, page.Hash, page.Page)
		}
		if page.Page.Offset != len(ids) || page.Page.ReturnedCount != len(page.Entries) || len(page.Entries) == 0 {
			t.Fatalf("page %d = %+v with %d entries, after %d facets", pages+1, page.Page, len(page.Entries), len(ids))
		}
		if n := encoded(t, page.Entries); n > cfg.MaxReadBytes {
			t.Errorf("page %d encodes to %d bytes, over the bound %d", pages+1, n, cfg.MaxReadBytes)
		}
		ids = append(ids, idsOf(page.Entries)...)
		cursors = append(cursors, next)
		next = page.Page.NextCursor
	}
	if pages < 3 {
		t.Errorf("the read came in %d pages; the bound should cut it into more", pages)
	}
	if !slices.Equal(ids, pub.Receipt.FacetIDs) {
		t.Fatalf("the pages' union is %v,\nthe receipt names %v", ids, pub.Receipt.FacetIDs)
	}

	// The first page without a cursor is what the publish answered with, receipt and all.
	var first api.ReceiptShown
	call(t, "GET", base+"/v1/receipts/"+pub.ReceiptHash, testToken, "", http.StatusOK, &first)
	if first.Receipt == nil || first.Page == nil || first.Page.NextCursor != pub.Page.NextCursor || !slices.Equal(idsOf(first.Entries), idsOf(pub.Entries)) {
		t.Errorf("GET without a cursor = %+v, want the publish's first page", first.Page)
	}

	// A second receipt, of the same read cut to a field: a frame pages by rows.
	var framed api.Published
	call(t, "POST", base+"/v1/receipts?zone=UTC&projection=weights&fields=/kg", testToken, "", http.StatusCreated, &framed)
	if framed.Frame == nil || framed.Page.NextCursor == "" || framed.Page.TotalCount != 9 {
		t.Fatalf("the framed publish: frame %v, page %+v", framed.Frame != nil, framed.Page)
	}
	rows := framed.Frame.Len()
	for next := framed.Page.NextCursor; next != ""; {
		var page api.ReceiptShown
		call(t, "GET", base+"/v1/receipts/"+framed.ReceiptHash+"?cursor="+url.QueryEscape(next), testToken, "", http.StatusOK, &page)
		if page.Frame == nil || len(page.Frame.Columns) != len(framed.Frame.Columns) {
			t.Fatalf("a frame page lost its columns: %+v", page.Frame)
		}
		rows += page.Frame.Len()
		next = page.Page.NextCursor
	}
	if rows != 9 {
		t.Errorf("the frame's pages hold %d rows, the receipt 9", rows)
	}

	refused := []struct{ name, cursor string }{
		{"another receipt's", framed.Page.NextCursor},
		{"not a cursor", "nonsense"},
		{"an offset moved", forge(t, cursors[0], `"offset":`, fmt.Sprintf(`"offset":%d`, pub.Page.ReturnedCount+1))},
		{"an offset past the end", forge(t, cursors[0], `"offset":`, `"offset":99`)},
		{"another format", forge(t, cursors[0], `"v":`, `"v":2`)},
	}
	for _, tt := range refused {
		var e api.Error
		call(t, "GET", base+"/v1/receipts/"+pub.ReceiptHash+"?cursor="+url.QueryEscape(tt.cursor), testToken, "", http.StatusBadRequest, &e)
		if e.Code != api.CodeCursorInvalid {
			t.Errorf("%s cursor: code %q, %s; want cursor_invalid", tt.name, e.Code, e.Error)
		}
	}

	var ver api.ReceiptVerified
	call(t, "GET", base+"/v1/receipts/"+pub.ReceiptHash+"/verify", testToken, "", http.StatusOK, &ver)
	if !ver.Result.OK() {
		t.Errorf("the paged receipt does not verify: %+v", ver.Result)
	}
}

func idsOf(entries []fold.Entry) []string {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.FacetID)
	}
	return ids
}

// encoded is what entries encode to as the daemon answers them, the bound's measure.
func encoded(t *testing.T, entries []fold.Entry) int {
	t.Helper()
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(entries); err != nil {
		t.Fatal(err)
	}
	return b.Len() - 1
}

// forge rewrites one field of a cursor the daemon issued, as a client tampering with
// it would: the number after key becomes the one in with.
func forge(t *testing.T, cursor, key, with string) string {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(regexp.QuoteMeta(key) + `[0-9]+`)
	if !re.Match(b) {
		t.Fatalf("cursor %s has no %s", b, key)
	}
	return base64.RawURLEncoding.EncodeToString(re.ReplaceAll(b, []byte(with)))
}
