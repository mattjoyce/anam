package config_test

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/presentation"
	"github.com/mattjoyce/ashlar"
)

func TestInspectorStrictSchema(t *testing.T) {
	t.Parallel()
	tests := []struct{ yaml, pointer string }{
		{"[]", "/inspector"},
		{"{track_type: place.track}", "/inspector/track_type"},
		{"{unknown: []}", "/inspector/unknown"},
		{"{domains: {}}", "/inspector/domains"},
		{"{families: 1}", "/inspector/families"},
		{"{types: false}", "/inspector/types"},
		{"{domains: [null]}", "/inspector/domains/0"},
		{"{families: [null]}", "/inspector/families/0"},
		{"{types: [null]}", "/inspector/types/0"},
		{"{domains: [{}]}", "/inspector/domains/0/id"},
		{"{domains: [{id: null}]}", "/inspector/domains/0/id"},
		{"{families: [{}]}", "/inspector/families/0/id"},
		{"{families: [{id: null}]}", "/inspector/families/0/id"},
		{"{types: [{shape: star}]}", "/inspector/types/0/type"},
		{"{types: [{type: null, shape: star}]}", "/inspector/types/0/type"},
		{"{types: [{type: custom.test}]}", "/inspector/types/0/shape"},
		{"{types: [{type: custom.test, shape: null}]}", "/inspector/types/0/shape"},
		{"{domains: [{id: Upper}]}", "/inspector/domains/0/id"},
		{"{domains: [{id: ' trailing'}]}", "/inspector/domains/0/id"},
		{"{domains: [{id: bad-}]}", "/inspector/domains/0/id"},
		{"{families: [{id: other}]}", "/inspector/families/0/id"},
		{"{families: [{id: a.b}]}", "/inspector/families/0/id"},
		{"{families: [{id: okay, label: null}]}", "/inspector/families/0/label"},
		{"{families: [{id: okay, label: ''}]}", "/inspector/families/0/label"},
		{"{families: [{id: okay, label: '  '}]}", "/inspector/families/0/label"},
		{"{families: [{id: okay, hue: 2}]}", "/inspector/families/0/hue"},
		{"{domains: [{id: okay, label: wrong}]}", "/inspector/domains/0/label"},
		{"{types: [{type: custom.test, shape: star, extra: 1}]}", "/inspector/types/0/extra"},
		{"{types: [{type: invalid, shape: star}]}", "/inspector/types/0/type"},
		{"{types: [{type: custom.test, shape: '<svg>'}]}", "/inspector/types/0/shape"},
		{"{types: [{type: custom.test, shape: star, legacy_kind: null}]}", "/inspector/types/0/legacy_kind"},
		{"{types: [{type: custom.test, shape: star, legacy_kind: ''}]}", "/inspector/types/0/legacy_kind"},
		{"{types: [{type: custom.test, shape: star, legacy_kind: unknown}]}", "/inspector/types/0/legacy_kind"},
		{"{types: [{type: custom.test, shape: star, legacy_family: null}]}", "/inspector/types/0/legacy_family"},
		{"{types: [{type: custom.test, shape: star, legacy_family: okay}]}", "/inspector/types/0/legacy_family"},
		{"{types: [{type: custom.test, shape: star, legacy_kind: event, legacy_family: other}]}", "/inspector/types/0/legacy_family"},
		{"{types: [{type: custom.test, shape: star, legacy_kind: event, legacy_family: '-bad'}]}", "/inspector/types/0/legacy_family"},
		{"{domains: [{id: a}, {id: a}]}", "/inspector/domains/1/id"},
		{"{families: [{id: a}, {id: a}]}", "/inspector/families/1/id"},
		{"{types: [{type: custom.test, shape: star}, {type: custom.test, shape: cross}]}", "/inspector/types/1/type"},
	}
	for _, hue := range []string{"-2", "8", "'0'", "1.5", "true"} {
		tests = append(tests, struct{ yaml, pointer string }{"{domains: [{id: okay, hue: " + hue + "}]}", "/inspector/domains/0/hue"})
	}
	for i, tt := range tests {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninclude: sub/child.yaml\n", "sub/child.yaml": "include: nested.yaml\n", "sub/nested.yaml": "# effective value\ninspector: " + tt.yaml + "\n"})
			c, err := config.Load(file)
			if err == nil {
				t.Fatalf("accepted %s", tt.yaml)
			}
			if !errors.Is(err, config.ErrInvalid) || !strings.Contains(err.Error(), tt.pointer) || !strings.Contains(err.Error(), "sub/nested.yaml:2") {
				t.Fatalf("diagnostic lost pointer or origin: %v", err)
			}
			if len(c.Presentation().Domains())+len(c.Presentation().Families())+len(c.Presentation().Overrides()) != 0 {
				t.Fatal("usable partial value")
			}
		})
	}
}

func TestInspectorAcceptedValues(t *testing.T) {
	t.Parallel()
	for _, hue := range []string{"", ", hue: null", ", hue: -1", ", hue: 0", ", hue: 7"} {
		t.Run(hue, func(t *testing.T) {
			t.Parallel()
			file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninspector:\n  domains: [{id: 1--domain" + hue + "}]\n  families: [{id: 1--family}, {id: custom, label: ' <Visible> '}]\n  types:\n" + shapeOverrides()})
			c, err := config.Load(file)
			if err != nil {
				t.Fatal(err)
			}
			p := c.Presentation()
			want := "hx"
			if strings.HasSuffix(hue, "0") {
				want = "h0"
			}
			if strings.HasSuffix(hue, "7") {
				want = "h7"
			}
			if p.Hue("1--domain") != want || p.Families()[0].Label != "1--family" || p.Families()[1].Label != " <Visible> " || len(p.Overrides()) != 8 {
				t.Fatalf("wrong compiled value %+v", p)
			}
		})
	}
}

func shapeOverrides() string {
	var b strings.Builder
	for _, shape := range []string{"circle", "square", "diamond", "tri-up", "tri-down", "cross", "star", "alert"} {
		fmt.Fprintf(&b, "    - {type: dormant.%s, shape: %s}\n", shape, shape)
	}
	return b.String()
}

func TestInspectorNullComposition(t *testing.T) {
	t.Parallel()
	arrays := map[string]string{"domains": "[{id: custom, hue: 0}]", "families": "[{id: custom}]", "types": "[{type: custom.test, shape: star}]"}
	for section, array := range arrays {
		for _, pair := range [][2]string{{array, "null"}, {"null", array}, {"null", "null"}, {"null", "[]"}} {
			t.Run(section+pair[0]+pair[1], func(t *testing.T) {
				t.Parallel()
				file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninspector:\n  " + section + ": " + pair[0] + "\ninclude: child.yaml\n", "child.yaml": "# final layer\ninspector:\n  " + section + ": " + pair[1] + "\n"})
				r, err := config.LoadResolved(file)
				if err != nil {
					t.Fatal(err)
				}
				p := r.Value().Presentation()
				n := len(p.Domains()) + len(p.Families()) + len(p.Overrides())
				want := 0
				if pair[1] == array {
					want = 1
				}
				if n != want {
					t.Fatalf("%s has %d entries, want %d", section, n, want)
				}
				origin, ok := r.ProvenanceAt(ashlar.Pointer("/inspector/" + section))
				if !ok || origin.Effective.Span.Source != "child.yaml" || origin.Effective.Span.Start.Line != 3 {
					t.Fatalf("origin = %+v", origin)
				}
				if want == 1 {
					switch section {
					case "domains":
						if p.Domains()[0] != (presentation.Domain{ID: "custom", Hue: 0}) {
							t.Fatal("domain value lost")
						}
					case "families":
						if p.Families()[0] != (presentation.Family{ID: "custom", Label: "custom"}) {
							t.Fatal("family value lost")
						}
					case "types":
						if p.Overrides()[0] != (presentation.Override{Type: "custom.test", Shape: "star"}) {
							t.Fatal("type value lost")
						}
					}
					origin, _ = r.ProvenanceAt(ashlar.Pointer("/inspector/" + section + "/0"))
					if origin.Effective.Span.Source != "child.yaml" || origin.Effective.Span.Start.Line != 3 {
						t.Fatalf("entry origin = %+v", origin)
					}
				}
			})
		}
	}
}

func TestInspectorLayers(t *testing.T) {
	t.Parallel()
	file := includeFiles(t, map[string]string{
		"config.yaml": "subject_id: test\ninclude: [child.yaml, local.yaml]\ninspector: {domains: [{id: root}], families: [{id: duplicate}, {id: duplicate}]}\n",
		"child.yaml":  "include: nested.yaml\ninspector: {domains: [{id: child}]}\n",
		"nested.yaml": "inspector: {domains: [{id: nested}], families: [{id: nested}], types: [{type: custom.test, shape: star}]}\n",
		"local.yaml":  "inspector: {domains: [{id: local}], families: [], types: null}\n",
	})
	r, err := config.LoadResolved(file)
	if err != nil {
		t.Fatal(err)
	}
	p := r.Value().Presentation()
	if !reflect.DeepEqual(p.Domains(), []presentation.Domain{{ID: "local", Hue: -1}}) || len(p.Families())+len(p.Overrides()) != 0 {
		t.Fatalf("wrong replacement %+v", p)
	}
	for _, child := range []string{"inspector: null", "inspector: {}"} {
		file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninclude: child.yaml\ninspector: {domains: [{id: kept}]}\n", "child.yaml": child})
		c, err := config.Load(file)
		if err != nil || c.Presentation().Domains()[0].ID != "kept" {
			t.Fatalf("top null/empty mapping cleared preferences: %v", err)
		}
	}
	for _, section := range []string{"domains", "families", "types"} {
		file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninclude: child.yaml\ninspector: {" + section + ": []}", "child.yaml": "inspector: {" + section + ": {}}"})
		if _, err := config.Load(file); !errors.Is(err, ashlar.ErrComposition) {
			t.Fatalf("kind conflict accepted: %v", err)
		}
	}
}

func TestInspectorYAMLOnly(t *testing.T) {
	// Changes process environment to verify structured environment input stays unbound.
	t.Setenv("ANAM_INSPECTOR", "invalid structured input")
	file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\ninspector: {}"})
	if _, err := config.Load(file); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadOverriding(file, map[string]string{"inspector": "{}"}); err == nil {
		t.Fatal("accepted structured string override")
	}
}

func TestInspectorAbsentDefaults(t *testing.T) {
	t.Parallel()
	for _, setting := range []string{"", "inspector: null", "inspector: {}"} {
		file := includeFiles(t, map[string]string{"config.yaml": "subject_id: test\n" + setting})
		c, err := config.Load(file)
		if err != nil {
			t.Fatal(err)
		}
		p := c.Presentation()
		if p.Hue("health") != "hx" || len(p.Domains())+len(p.Families())+len(p.Overrides()) != 0 {
			t.Fatal("absent policy carries personal defaults")
		}
	}
}
