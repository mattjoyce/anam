package inspect_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Card 165: a record.link is a file Anam remembers but does not hold. Its page says
// so and offers no download, even with a library configured and a sha256 in the
// payload: the sha256 names bytes outside Anam, and nothing is served from anywhere.
func TestFacet_LinkSaysBytesNotHeldAndOffersNoDownload(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	l, err := ledger.Create(ctx, filepath.Join(t.TempDir(), "ledger.db"), person)
	if err != nil {
		t.Fatalf("ledger.Create: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	sum := strings.Repeat("ab", 32)
	link := seedFacet(t, facet.SourceRef("links", "path:vault/notes/2026-01-30 referral.md"), "record.link",
		`{"id": "path:vault/notes/2026-01-30 referral.md", "identity": "path", "root": "vault", "path": "notes/2026-01-30 referral.md", "sha256": "`+sum+`", "media_type": "text/markdown", "bytes": 18, "date": "2026-01-30T00:00:00+11:00", "date_from": "frontmatter"}`)
	if err := l.Admit(ctx, 0, link, event(facet.EventAdmitted, link, 1)); err != nil {
		t.Fatalf("Admit: %v", err)
	}
	srv := httptest.NewServer(inspect.Handler(inspect.Deps{Catalog: testCatalog(), Ledger: l, Timeout: 30 * time.Second, Buckets: defaultBuckets, Library: t.TempDir()}))
	t.Cleanup(srv.Close)

	code, body := get(t, srv.URL+"/facet?id="+link.ID)
	if code != http.StatusOK {
		t.Fatalf("facet page = %d:\n%s", code, body)
	}
	if strings.Contains(body, "/document?") {
		t.Errorf("a record.link page offers a document link:\n%s", body)
	}
	for _, want := range []string{"The bytes are not held", "a file Anam remembers but does not hold"} {
		if !strings.Contains(body, want) {
			t.Errorf("facet page does not say %q:\n%s", want, body)
		}
	}
}
