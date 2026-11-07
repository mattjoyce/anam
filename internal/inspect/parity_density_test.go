package inspect

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestParityRunsAndTally(t *testing.T) {
	t.Parallel()
	var seeds []paritySeed
	for _, group := range []struct {
		typ   string
		count int
	}{{"health.weight", 1501}, {"record.link", 3}, {"org.employment.role-changed", 3}} {
		for range group.count {
			seeds = append(seeds, paritySeed{group.typ, 2})
		}
	}
	l := parityLedger(t, seeds)
	for _, c := range []struct {
		name, query string
		opened      bool
		filtered    bool
	}{
		{"collapsed", "", false, false},
		{"opened", "&open=health&open=record&open=org", true, false},
		{"filtered", "&type=health.weight", true, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			page := parityPage(t, l, nil, c.query)
			band := page.one(t, "svg", "band")
			parityCheckRuns(t, band, c.opened, c.filtered)
			parityCheckTally(t, page, c.filtered)
			if !strings.Contains(parityText(page), "W = 1507") {
				t.Error("dense fixture watermark changed, want W = 1507")
			}
		})
	}
}

func parityCheckRuns(t *testing.T, band *parityNode, opened, filtered bool) {
	t.Helper()
	want := map[string]struct{ count, class, shapeName string }{
		"health.weight":               {"1501", "h7 hollow", "square"},
		"record.link":                 {"3", "h0", "cross"},
		"org.employment.role-changed": {"3", "h5", "diamond"},
	}
	if filtered {
		delete(want, "record.link")
		delete(want, "org.employment.role-changed")
	}
	runs := band.find("a", "run")
	if len(runs) != len(want) || len(band.find("a", "pt")) != 0 {
		t.Fatalf("dense run/point counts got %d/%d want %d/0", len(runs), len(band.find("a", "pt")), len(want))
	}
	for _, run := range runs {
		parityCheckRun(t, run, want, opened)
	}
}

func parityCheckRun(t *testing.T, run *parityNode, want map[string]struct{ count, class, shapeName string }, opened bool) {
	t.Helper()
	u, err := url.Parse(run.Attr["href"])
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	typ := q.Get("type")
	w, ok := want[typ]
	if !ok {
		t.Fatalf("unexpected run type %s", typ)
	}
	if title := parityText(run.one(t, "title", "")); title != w.count+" × "+typ+" · all at 2026-01-02 00:00Z · click to list them" {
		t.Errorf("%s run title/count changed, got %q", typ, title)
	}
	if u.Path != "/select" || q.Get("from") != "2026-01-02T00:00:00.000000000Z" || q.Get("to") != "2026-01-02T00:00:00.000000001Z" {
		t.Errorf("%s moment run link changed, got %s", typ, u)
	}
	line := run.one(t, "line", "")
	if line.Attr["class"] != w.class || parityNumber(t, line, "x1") != 120 || parityNumber(t, line, "x2") != 120 {
		t.Errorf("%s run line hue/fill/position changed, got %v", typ, line.Attr)
	}
	y := map[string]float64{"health.weight": 45, "record.link": 75, "org.employment.role-changed": 105}[typ]
	if opened {
		y = map[string]float64{"health.weight": 75, "record.link": 135, "org.employment.role-changed": 195}[typ]
	}
	if got := parityNumber(t, line, "y1"); got != y {
		t.Errorf("%s run y got %v want %v", typ, got, y)
	}
	paths := run.find("path", "")
	if len(paths) != 2 {
		t.Fatalf("%s run has %d endpoint paths, want 2", typ, len(paths))
	}
	for _, path := range paths {
		if path.Attr["class"] != w.class {
			t.Errorf("%s run hollow/fill or hue got %q want %q", typ, path.Attr["class"], w.class)
		}
		parityCheckPath(t, typ+" run endpoint", path.Attr["d"], w.shapeName, 120, y)
	}

}

func parityCheckTally(t *testing.T, page *parityNode, filtered bool) {
	t.Helper()
	var got []string
	for _, entry := range page.one(t, "div", "tally").find("a", "") {
		u, err := url.Parse(entry.Attr["href"])
		if err != nil {
			t.Fatal(err)
		}
		typ := u.Query().Get("type")
		got = append(got, typ+" "+entry.one(t, "span", "sw").Attr["class"]+" "+parityText(entry.one(t, "span", "dim")))
	}
	// The base renders tally types in first-facet order, not legend order.
	want := []string{"health.weight sw h7 (1501)", "org.employment.role-changed sw h5 (3)", "record.link sw h0 (3)"}
	if filtered {
		want = want[:1]
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tally order/counts/hues changed\ngot  %v\nwant %v", got, want)
	}
}
