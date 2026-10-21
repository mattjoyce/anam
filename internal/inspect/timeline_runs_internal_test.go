package inspect

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/facet"
)

// A run whose marks share one instant cannot be split by zooming, so it lists them
// on the selection page; a run with any spread still zooms in.
func TestMakeRun_OneInstantListsAndASpreadZooms(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC)
	mark := func(d time.Duration) laneMark {
		return laneMark{point: tlPoint{X: 100}, at: at.Add(d), typ: facet.Type("health.heart-rate-day")}
	}
	cases := []struct {
		name   string
		group  []laneMark
		prefix string
		title  string
	}{
		{"one instant", []laneMark{mark(0), mark(0), mark(0)}, "/select?", "all at 2026-10-19 23:00Z · click to list them"},
		{"spread", []laneMark{mark(0), mark(time.Hour), mark(2 * time.Hour)}, "/?", "click to zoom in"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := makeRun(c.group, tlView{})
			if !strings.HasPrefix(r.Href, c.prefix) || !strings.Contains(r.Title, c.title) {
				t.Fatalf("href %q title %q", r.Href, r.Title)
			}
			if c.prefix != "/select?" {
				return
			}
			q, err := url.ParseQuery(strings.TrimPrefix(r.Href, "/select?"))
			if err != nil {
				t.Fatal(err)
			}
			if q.Get("type") != "health.heart-rate-day" || q.Get("from") != "2026-10-19T23:00:00.000000000Z" ||
				q.Get("to") != "2026-10-19T23:00:00.000000001Z" {
				t.Errorf("query %v", q)
			}
		})
	}
}
