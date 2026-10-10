package inspect

import (
	"fmt"
	"testing"

	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The facet list filters, pages newest first, and links to the pages either side
// with the filter kept.
func TestPageFacets(t *testing.T) {
	t.Parallel()
	var rows []facetRow
	for i := 0; i < 700; i++ {
		typ, src := facet.Type("health.weight"), "records"
		if i%2 == 1 {
			typ, src = "health.sleep", "health-db"
		}
		rows = append(rows, facetRow{ID: fmt.Sprintf("f%04d", i), Type: typ, Source: src,
			FactTime: fmt.Sprintf("2026-01-01T00:%02d:%02dZ", i/60, i%60), Status: facet.StatusAdmitted})
	}
	all := pageFacets(rows, facetFilter{}, 0)
	if all.Matched != 700 || len(all.Rows) != listPage || all.Rows[0].ID != "f0699" || all.Older == "" || all.Newer != "" {
		t.Fatalf("first page: matched %d, %d rows, first %s, older %q newer %q", all.Matched, len(all.Rows), all.Rows[0].ID, all.Older, all.Newer)
	}
	sleep := pageFacets(rows, facetFilter{Type: "health.sleep"}, 300)
	if sleep.Matched != 350 || len(sleep.Rows) != 50 || sleep.From != 301 || sleep.Older != "" ||
		sleep.Newer != "/facets?type=health.sleep" {
		t.Fatalf("sleep page 2: %+v", sleep)
	}
	if got := pageFacets(rows, facetFilter{Source: "records", Status: "tombstoned"}, 0); got.Matched != 0 {
		t.Errorf("no tombstoned records facet, matched %d", got.Matched)
	}
	if beyond := pageFacets(rows, facetFilter{}, 9999); len(beyond.Rows) != 0 {
		t.Errorf("a skip past the end showed %d rows", len(beyond.Rows))
	}
}

// The events page is the listPage events up to before, newest first.
func TestEventPage(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		head        ledger.Watermark
		before      int64
		since, upto ledger.Watermark
	}{
		{3, 0, 0, 3},
		{1000, 0, 700, 1000},
		{1000, 701, 400, 700},
		{1000, 250, 0, 249},
		{1000, 5000, 700, 1000},
	} {
		since, upto := eventPage(tc.head, tc.before)
		if since != tc.since || upto != tc.upto {
			t.Errorf("eventPage(%d, %d) = (%d, %d], want (%d, %d]", tc.head, tc.before, since, upto, tc.since, tc.upto)
		}
	}
}
