package inspect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// linkedRecord holds two weigh-ins and a note about the first, admitted to a real
// ledger and replayed into a real operational store under the test's own
// directories, so the facet page reads its links from the same index the API does.
type linkedRecord struct {
	l           *ledger.Ledger
	db          *ops.DB
	weigh, note string
	alone       string // the second weigh-in, which nothing names
	w           int64  // the head, and the index's cursor once synced
}

func newLinkedRecord(t *testing.T) linkedRecord {
	t.Helper()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	db, err := ops.Open(ctx, filepath.Join(t.TempDir(), "tracking.db"), defaultBuckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	weigh := seedFacet(t, "scales:2026-10-09 07:30", "health.weight", `{"id": "2026-10-09 07:30", "date": "2026-10-09 07:30", "kg": 80.5}`)
	alone := seedFacet(t, "scales:2026-10-10 07:30", "health.weight", `{"id": "2026-10-10 07:30", "date": "2026-10-10 07:30", "kg": 80.1}`)
	note := seedFacet(t, "records:note:1", string(catchup.EdgeType),
		`{"id": "note:1", "at": "2026-10-09 11:29", "to": ["`+weigh.ID+`"], "text": "Fasted, after the run."}`)
	var seq int64
	for _, f := range []facet.Facet{weigh, alone, note} {
		if err := l.Admit(ctx, ledger.Watermark(seq), f, event(facet.EventAdmitted, f, seq+1)); err != nil {
			t.Fatalf("Admit %s: %v", f.Provenance.SourceRef, err)
		}
		seq++
	}
	if _, err := catchup.Sync(ctx, l, db, defaultBuckets); err != nil {
		t.Fatalf("catchup.Sync: %v", err)
	}
	return linkedRecord{l: l, db: db, weigh: weigh.ID, note: note.ID, alone: alone.ID, w: seq}
}

// The facet page lists the live edges touching the facet, read from the edge index
// through the same code as the API's links part: the note about a weigh-in shows its
// kind, which side the weigh-in is on, when it was asserted, its text from the ledger,
// and the edge itself as a link. A facet nothing names says so.
func TestFacet_ListsTheEdgesTouchingIt(t *testing.T) {
	t.Parallel()
	rec := newLinkedRecord(t)
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: rec.l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Edges: rec.db}))
	t.Cleanup(srv.Close)

	code, body := get(t, srv.URL+"/facet?id="+rec.weigh)
	if code != http.StatusOK {
		t.Fatalf("GET /facet = %d\n%s", code, body)
	}
	for _, want := range []string{
		"<h2>links <span class=\"dim\">live edges touching this facet, from the edge index at W = 3</span></h2>",
		"<td>about</td>",
		"<td>to</td>",
		"<td class=\"mono\"><strong>this facet</strong></td>",
		"<td>Fasted, after the run.</td>",
		`<a class="mono" href="/facet?id=` + rec.note + `">` + rec.note + `</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the weigh-in's page lacks %q", want)
		}
	}
	if strings.Contains(body, "no edges touch this facet") {
		t.Error("the weigh-in's page says no edges touch it")
	}

	code, body = get(t, srv.URL+"/facet?id="+rec.alone)
	if code != http.StatusOK {
		t.Fatalf("GET /facet (alone) = %d", code)
	}
	if !strings.Contains(body, "no edges touch this facet.") || strings.Contains(body, "<td>about</td>") {
		t.Errorf("the unnamed weigh-in's page should say no edges touch it:\n%s", body)
	}
}

// Without an edge index (a standalone inspector over a bare ledger) the page says
// there is none to read, rather than draw the facet as if nothing touched it.
func TestFacet_SaysWhenThereIsNoEdgeIndex(t *testing.T) {
	t.Parallel()
	rec := newLinkedRecord(t)
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: rec.l, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(srv.Close)
	code, body := get(t, srv.URL+"/facet?id="+rec.weigh)
	if code != http.StatusOK {
		t.Fatalf("GET /facet = %d", code)
	}
	if !strings.Contains(body, "no edge index to read") || strings.Contains(body, "no edges touch this facet") {
		t.Errorf("without an index the page should say so:\n%s", body)
	}
}

// A facet id inside a payload gets a link icon after it, in the verbatim payload and in
// the field table, opening that facet; a string that is not an id gets none, and the
// payload text is as it was (card 244).
func TestFacetPage_APayloadIDGetsALinkIcon(t *testing.T) {
	t.Parallel()
	rec := newLinkedRecord(t)
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: rec.l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Edges: rec.db}))
	t.Cleanup(srv.Close)

	code, body := get(t, srv.URL+"/facet?id="+rec.note)
	if code != http.StatusOK {
		t.Fatalf("GET /facet = %d\n%s", code, body)
	}
	icon := `<a class="openid" href="/facet?id=` + rec.weigh + `"`
	_, payload, ok := strings.Cut(body, `<pre class="json mono">`)
	if !ok {
		t.Fatalf("no payload on the page\n%s", body)
	}
	payload, _, _ = strings.Cut(payload, "</pre>")
	if n := strings.Count(payload, icon); n != 1 {
		t.Errorf("payload draws the link icon %d times, want once\n%s", n, payload)
	}
	if n := strings.Count(payload, `class="openid"`); n != 1 {
		t.Errorf("payload draws %d icons, want one: the text and the source ref are not ids\n%s", n, payload)
	}
	// The id is still text in the payload, the icon is a separate anchor after it.
	if want := `<span class="j-str">&#34;` + rec.weigh + `&#34;</span>` + icon; !strings.Contains(payload, want) {
		t.Errorf("payload does not read %q", want)
	}
	if !strings.Contains(payload, `<span class="j-str">&#34;Fasted, after the run.&#34;</span><span class="j-pun">,</span>`) {
		t.Errorf("payload text changed\n%s", payload)
	}
	// A facet with no id in it draws no icon.
	_, body = get(t, srv.URL+"/facet?id="+rec.weigh)
	if _, p, _ := strings.Cut(body, `<pre class="json mono">`); strings.Contains(p, "openid") {
		t.Errorf("a payload with no facet id draws an icon")
	}
}

// The field table links an id too: the note's `to` is an array, drawn as its JSON with
// the icon after each id it holds (card 244).
func TestFacetPage_AFieldTableIDGetsALinkIcon(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	cur, err := def.Parse(bundledBody(t, catchup.EdgeType))
	if err != nil {
		t.Fatal(err)
	}
	const a, b = "bcbf563c-d051-5bfd-85f6-77135ef71daa", "594d7e55-f749-5c53-bc16-b05fdc9e88a9"
	note := seedFacet(t, "records:note:2", string(catchup.EdgeType),
		`{"id": "note:2", "at": "2026-10-09 11:29", "to": ["`+a+`", "`+b+`"], "text": "two ends"}`)
	note.DefinitionVersion = cur.Version
	if err := l.Admit(ctx, 0, note, event(facet.EventAdmitted, note, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets}))
	t.Cleanup(srv.Close)

	code, body := get(t, srv.URL+"/facet?id="+note.ID)
	if code != http.StatusOK {
		t.Fatalf("GET /facet = %d\n%s", code, body)
	}
	table, _, ok := strings.Cut(body, "<h2>payload")
	if !ok || !strings.Contains(table, `<table class="fields">`) {
		t.Fatalf("no field table\n%s", body)
	}
	for _, id := range []string{a, b} {
		icon := `<a class="openid" href="/facet?id=` + id + `"`
		if n := strings.Count(table, icon); n != 1 {
			t.Errorf("field table links %s %d times, want once", id, n)
		}
		if n := strings.Count(body, icon); n != 2 {
			t.Errorf("page links %s %d times, want twice (table and payload)", id, n)
		}
	}
	if n := strings.Count(table, `class="openid"`); n != 2 {
		t.Errorf("field table draws %d icons, want 2: the other values are not ids", n)
	}
}
