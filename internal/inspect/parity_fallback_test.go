package inspect

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/def"
)

func TestParityHistoricalFallback(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, typ, family, shapeName, class string
		definition                          *def.Definition
		nilRegistry                         bool
	}{
		{"nil registry", "health.weight", "body", "square", "h7 hollow", nil, true},
		{"missing Definition", "health.blood-pressure", "vitals", "square", "h7 hollow", nil, false},
		{"retired override", "health.metric-group", "movement", "circle", "h7", nil, false},
		{"incomplete metadata", "health.sleep", "sleep", "circle", "h7", &def.Definition{Type: "health.sleep"}, false},
		{"family disagrees", "health.weight", "garden", "square", "h7 hollow", &def.Definition{Type: "health.weight", Family: "garden", Kind: def.KindReading}, false},
		{"kind disagrees", "health.weight", "body", "diamond", "h7", &def.Definition{Type: "health.weight", Kind: def.KindEvent}, false},
		{"family and kind disagree", "health.blood-pressure", "notes", "diamond", "h7", &def.Definition{Type: "health.blood-pressure", Family: "notes", Kind: def.KindEvent}, false},
		{"family without kind", "health.donation", "notes", "star", "h7", &def.Definition{Type: "health.donation", Family: "notes"}, false},
		{"retired name without override", "legacy.gone", "other", "circle", "hx", nil, false},
		{"unlisted declared reading", "health.instrument", "garden", "circle", "h7 hollow", &def.Definition{Type: "health.instrument", Family: "garden", Kind: def.KindReading}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var reg *def.Registry
			if !c.nilRegistry {
				var definitions []def.Definition
				if c.definition != nil {
					definitions = append(definitions, *c.definition)
				}
				var err error
				reg, err = def.NewRegistry(definitions...)
				if err != nil {
					t.Fatal(err)
				}
			}
			l := parityLedger(t, []paritySeed{{c.typ, 2}})
			domain, _, _ := strings.Cut(c.typ, ".")
			page := parityPage(t, l, reg, "&open="+domain)
			band := page.one(t, "svg", "band")
			var rows []string
			for _, row := range band.find("text", "rowlabel") {
				rows = append(rows, parityText(row))
			}
			if want := []string{"▾ " + domain, c.family}; !reflect.DeepEqual(rows, want) {
				t.Errorf("%s fallback rows got %v want %v", c.typ, rows, want)
			}
			point := band.one(t, "a", "pt")
			path := point.one(t, "path", "")
			if path.Attr["class"] != c.class {
				t.Errorf("%s fallback hollow/fill or hue got %q want %q", c.typ, path.Attr["class"], c.class)
			}
			parityCheckPath(t, c.typ, path.Attr["d"], c.shapeName, 120, 75)
			group := page.one(t, "fieldset", "fam")
			if label := parityText(group.one(t, "legend", "")); label != c.family {
				t.Errorf("%s fallback legend family got %s want %s", c.typ, label, c.family)
			}
			swatch := group.one(t, "path", "")
			if swatch.Attr["class"] != c.class || swatch.Attr["d"] != parityPaths()[c.shapeName] {
				t.Errorf("%s fallback legend mark changed, got %v", c.typ, swatch.Attr)
			}
			if !strings.Contains(parityText(page), "W = 1") || point.hasClass("dead") {
				t.Error("fallback is not drawn live at the pinned watermark before the tombstone")
			}
		})
	}
}
