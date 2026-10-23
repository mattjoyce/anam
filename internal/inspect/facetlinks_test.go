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
