package inspect

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type parityGroup struct {
	Domain, Label, Hue string
	Types              []string
}

// Order and labels are literal, independent of rowsFor, lanesFor and the tables.
func pinnedParityGroups() []parityGroup {
	return []parityGroup{
		{"health", "vitals", "h7", []string{"health.heart-rate", "health.zz-measure", "health.aa-reading", "health.blood-pressure", "health.aa-event", "health.heart-rate-alert"}},
		{"health", "body", "h7", []string{"health.measurement.withings", "health.weight"}},
		{"health", "sleep", "h7", []string{"health.sleep"}},
		{"health", "movement", "h7", []string{"health.metric-group"}},
		{"health", "labs", "h7", []string{"health.lab-panel"}},
		{"health", "conditions", "h7", []string{"health.illness.ended", "health.illness.started"}},
		{"health", "meds", "h7", []string{"health.medication.dose", "health.medication.dose-missed", "health.medication.started", "health.medication.stopped"}},
		{"health", "intake", "h7", []string{"health.intake"}},
		{"health", "clinical", "h7", []string{"health.donation", "health.encounter"}},
		{"health", "notes", "h7", []string{"health.note"}},
		{"health", "alpha", "h7", []string{"health.alpha"}},
		{"health", "zeta", "h7", []string{"health.zeta"}},
		{"health", "other", "h7", []string{"health.unknown"}},
		{"activity", "exercise", "h3", []string{"activity.gym-visit"}},
		{"place", "location", "h1", []string{"place.sample"}},
		{"place", "trips", "h1", []string{"place.trip.arrived", "place.trip.departed", "place.trip.flight"}},
		{"record", "documents", "h0", []string{"record.document", "record.link"}},
		{"mind", "attention", "h6", []string{"mind.attention.ended", "mind.attention.started"}},
		{"org", "employment", "h5", []string{"org.employment.ended", "org.employment.role-changed", "org.employment.started"}},
		{"money", "purchases", "h2", []string{"money.purchase"}},
		{"social", "other", "hx", []string{"social.sample"}},
		{"media", "other", "hx", []string{"media.sample"}},
		{"work", "other", "hx", []string{"work.sample"}},
		{"legacy", "other", "hx", []string{"legacy.sample"}},
	}
}

func paritySeeds() []paritySeed {
	var seeds []paritySeed
	for _, g := range pinnedParityGroups() {
		for _, typ := range g.Types {
			seeds = append(seeds, paritySeed{Type: typ})
		}
	}
	// Admission/time order deliberately opposes row and legend order.
	for i, j := 0, len(seeds)-1; i < j; i, j = i+1, j-1 {
		seeds[i], seeds[j] = seeds[j], seeds[i]
	}
	for i := range seeds {
		seeds[i].Day = i + 2
	}
	return seeds
}

func paritySymbol(typ string) (shapeName, suffix string) {
	if o, ok := pinnedParityGrammar().Overrides[typ]; ok {
		if o.Kind == "reading" {
			return o.Shape, " hollow"
		}
		return o.Shape, ""
	}
	switch typ {
	case "health.aa-reading", "health.alpha":
		return "circle", " hollow"
	case "health.aa-event", "health.intake", "health.note":
		return "diamond", ""
	default:
		return "circle", ""
	}
}

func TestParityTimeline(t *testing.T) {
	t.Parallel()
	seeds := paritySeeds()
	l, reg := parityLedger(t, seeds), parityRegistry(t)
	cases := []struct {
		name, query string
		opened      string
		filtered    bool
	}{
		{"collapsed", "", "", false},
		{"opened", "&open=health&open=activity&open=place&open=record&open=mind&open=org&open=money&open=social&open=media&open=work&open=legacy", "all", false},
		{"filtered", "&type=health.weight", "health", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			page := parityPage(t, l, reg, c.query)
			parityCheckPaint(t, page)
			band := page.one(t, "svg", "band")
			parityCheckRows(t, band, c.opened)
			parityCheckLegend(t, page, c.filtered)
			parityCheckPoints(t, band, seeds, c.opened, c.filtered)
			parityCheckStrip(t, page, seeds, c.filtered)
			if !strings.Contains(parityText(page), "W = 41") {
				t.Error("historical watermark changed, want W = 41 before the later tombstone")
			}
		})
	}
}

func parityCheckRows(t *testing.T, band *parityNode, opened string) {
	t.Helper()
	wantDomains := []string{"health", "activity", "place", "record", "mind", "org", "money", "social", "media", "work", "legacy"}
	wantHues := []string{"h7", "h3", "h1", "h0", "h6", "h5", "h2", "hx", "hx", "hx", "hx"}
	var gotDomains, gotRows, wantRows []string
	for _, d := range wantDomains {
		arrow := "▸ "
		if opened == "all" || opened == d {
			arrow = "▾ "
		}
		wantRows = append(wantRows, arrow+d)
		if opened == "all" || opened == d {
			for _, g := range pinnedParityGroups() {
				if g.Domain == d {
					wantRows = append(wantRows, g.Label)
				}
			}
		}
	}
	for i, d := range band.find("g", "domain") {
		gotDomains = append(gotDomains, d.Attr["data-domain"])
		if i < len(wantHues) && d.one(t, "rect", "lanebar").Attr["class"] != "lanebar "+wantHues[i] {
			t.Errorf("%s lane hue changed, want %s", d.Attr["data-domain"], wantHues[i])
		}
		if d.hasClass("open") != (opened == "all" || opened == d.Attr["data-domain"]) {
			t.Errorf("%s lane open state changed", d.Attr["data-domain"])
		}
	}
	for i, row := range band.find("text", "rowlabel") {
		gotRows = append(gotRows, parityText(row))
		if y := parityNumber(t, row, "y"); y != 45+30*float64(i) {
			t.Errorf("row %s y got %v want %v", parityText(row), y, 45+30*i)
		}
	}
	if !reflect.DeepEqual(gotDomains, wantDomains) {
		t.Errorf("domain order changed, got %v want %v", gotDomains, wantDomains)
	}
	if !reflect.DeepEqual(gotRows, wantRows) {
		t.Errorf("row order/labels changed\ngot  %v\nwant %v", gotRows, wantRows)
	}
}

func parityCheckLegend(t *testing.T, page *parityNode, filtered bool) {
	t.Helper()
	groups := page.find("fieldset", "fam")
	want := pinnedParityGroups()
	if len(groups) != len(want) {
		t.Fatalf("legend group count got %d want %d", len(groups), len(want))
	}
	for i, group := range groups {
		w := want[i]
		heading := group.one(t, "legend", "")
		if heading.Attr["title"] != w.Domain || parityText(heading) != w.Label {
			t.Errorf("legend group %d got %s/%s want %s/%s", i, heading.Attr["title"], parityText(heading), w.Domain, w.Label)
		}
		if hue := heading.one(t, "circle", "").Attr["class"]; hue != w.Hue {
			t.Errorf("legend %s/%s hue got %s want %s", w.Domain, w.Label, hue, w.Hue)
		}
		var types []string
		for _, label := range group.Children {
			if label.Tag != "label" {
				continue
			}
			input := label.one(t, "input", "")
			typ := input.Attr["value"]
			types = append(types, typ)
			shapeName, suffix := paritySymbol(typ)
			path := label.one(t, "path", "")
			if got := path.Attr["class"]; got != w.Hue+suffix {
				t.Errorf("legend %s hollow/fill or hue changed, got %q want %q", typ, got, w.Hue+suffix)
			}
			if got, wantPath := path.Attr["d"], parityPaths()[shapeName]; got != wantPath {
				t.Errorf("legend %s %s path changed, got %s want %s", typ, shapeName, got, wantPath)
			}
			_, checked := input.Attr["checked"]
			if checked != (!filtered || typ == "health.weight") || label.hasClass("off") == checked {
				t.Errorf("legend %s filter state changed", typ)
			}
			if count := parityText(label.one(t, "span", "dim")); count != "(1)" {
				t.Errorf("legend %s count got %s want (1)", typ, count)
			}
		}
		if !reflect.DeepEqual(types, w.Types) {
			t.Errorf("legend %s/%s type order changed, got %v want %v", w.Domain, w.Label, types, w.Types)
		}
	}
}

func parityCheckPoints(t *testing.T, band *parityNode, seeds []paritySeed, opened string, filtered bool) {
	t.Helper()
	points := band.find("a", "pt")
	wantCount := len(seeds)
	if filtered {
		wantCount = 1
	}
	if len(points) != wantCount || len(band.find("a", "run")) != 0 {
		t.Fatalf("point/run counts changed, got %d/%d want %d/0", len(points), len(band.find("a", "run")), wantCount)
	}
	byID := map[string]*parityNode{}
	for _, p := range points {
		byID[p.Attr["href"]] = p
	}
	centers := map[string]float64{}
	domain := ""
	for _, row := range band.find("text", "rowlabel") {
		label := parityText(row)
		if !row.hasClass("famlabel") {
			domain = strings.TrimSpace(strings.TrimLeft(label, "▸▾"))
			label = ""
		}
		centers[domain+"/"+label] = parityNumber(t, row, "y")
	}
	yOf := map[string]float64{}
	for i, seed := range seeds {
		if filtered && seed.Type != "health.weight" {
			continue
		}
		yOf[seed.Type] = parityCheckPoint(t, byID, centers, seed, i, opened)
	}
	if opened == "all" && !filtered {
		for _, g := range pinnedParityGroups() {
			for i := 1; i < len(g.Types); i++ {
				if yOf[g.Types[i-1]] >= yOf[g.Types[i]] {
					t.Errorf("%s/%s type sub-lane order changed, want %s above %s", g.Domain, g.Label, g.Types[i-1], g.Types[i])
				}
			}
		}
	}
}

func parityCheckPoint(t *testing.T, byID map[string]*parityNode, centers map[string]float64, seed paritySeed, i int, opened string) float64 {
	t.Helper()
	id := facetIDForParitySeed(i)
	p := byID["/facet?id="+id]
	if p == nil {
		t.Errorf("missing point %s", seed.Type)
		return 0
	}
	g := parityGroupFor(seed.Type)
	shapeName, suffix := paritySymbol(seed.Type)
	path := p.one(t, "path", "")
	if path.Attr["class"] != g.Hue+suffix {
		t.Errorf("point %s hollow/fill or hue changed, got %q want %q", seed.Type, path.Attr["class"], g.Hue+suffix)
	}
	x, y := parityNumber(t, p, "data-x"), parityNumber(t, p, "data-y")
	if x != 104+16*float64(seed.Day-1) {
		t.Errorf("%s x got %v want %v", seed.Type, x, 104+16*(seed.Day-1))
	}
	label := ""
	if opened == "all" || opened == g.Domain {
		label = g.Label
	}
	center, ok := centers[g.Domain+"/"+label]
	if !ok || y < center-9 || y > center+9 || (label == "" && y != center) {
		t.Errorf("%s y %v is outside its %s/%s row at %v", seed.Type, y, g.Domain, label, center)
	}
	parityCheckPath(t, seed.Type, path.Attr["d"], shapeName, x, y)
	return y
}

func parityGroupFor(typ string) parityGroup {
	for _, g := range pinnedParityGroups() {
		for _, candidate := range g.Types {
			if candidate == typ {
				return g
			}
		}
	}
	return parityGroup{}
}

func parityCheckStrip(t *testing.T, page *parityNode, seeds []paritySeed, filtered bool) {
	t.Helper()
	var got, want []string
	for _, a := range page.one(t, "div", "strip").find("a", "") {
		got = append(got, a.Attr["href"]+" "+a.Attr["class"])
	}
	for i, seed := range seeds {
		if !filtered || seed.Type == "health.weight" {
			want = append(want, fmt.Sprintf("/facet?id=%s %s", facetIDForParitySeed(i), parityGroupFor(seed.Type).Hue))
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("canonical strip order/hues changed\ngot  %v\nwant %v", got, want)
	}
}
