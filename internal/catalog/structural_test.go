package catalog_test

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// editDefinition rewrites one definition file of dir through its decoded JSON.
func editDefinition(t *testing.T, dir, name string, change func(m map[string]any)) {
	t.Helper()
	path := filepath.Join(dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	change(m)
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
}

// property is a schema property of a decoded definition.
func property(m map[string]any, name string) map[string]any {
	props := m["schema"].(map[string]any)["properties"].(map[string]any)
	p, _ := props[name].(map[string]any)
	return p
}

func dropProperty(m map[string]any, name string) {
	delete(m["schema"].(map[string]any)["properties"].(map[string]any), name)
}

// TestLoad_RefusesAMalformedStructuralType: the engine reads record.edge's from, to,
// about and kind, and record.document's sha256, by pointer. A catalog that declares
// one in a shape the consumer cannot read is refused at Load, naming the type, its
// version and the pointer. A pointer left undeclared is not a malformed one.
func TestLoad_RefusesAMalformedStructuralType(t *testing.T) {
	t.Parallel()
	const edge, doc = "record.edge.json", "record.document.json"
	tests := []struct {
		name   string
		file   string
		change func(m map[string]any)
		want   string // "" loads
	}{
		{"unchanged", edge, func(map[string]any) {}, ""},
		{"kind without its enum", edge, func(m map[string]any) { delete(property(m, "kind"), "enum") }, "record.edge v1 /kind"},
		{"kind with an empty enum", edge, func(m map[string]any) { property(m, "kind")["enum"] = []any{} }, "record.edge v1 /kind"},
		{"kind not declared", edge, func(m map[string]any) { dropProperty(m, "kind") }, ""},
		{"from a string", edge, func(m map[string]any) {
			p := property(m, "from")
			p["type"] = "string"
			delete(p, "items")
		}, "record.edge v1 /from"},
		{"to of numbers", edge, func(m map[string]any) {
			property(m, "to")["items"].(map[string]any)["type"] = "number"
		}, "record.edge v1 /to"},
		{"about with no item type", edge, func(m map[string]any) { delete(property(m, "about"), "items") }, "record.edge v1 /about"},
		{"about not declared", edge, func(m map[string]any) {
			dropProperty(m, "about")
			s := m["schema"].(map[string]any)
			delete(s, "not")
			s["anyOf"] = []any{map[string]any{"required": []any{"text"}}}
		}, ""},
		{"sha256 not required", doc, func(m map[string]any) {
			s := m["schema"].(map[string]any)
			s["required"] = slices.DeleteFunc(s["required"].([]any), func(v any) bool { return v == "sha256" })
		}, "record.document v1 /sha256"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			minDefs, minProjs := fixture.Minimal()
			defs, projs := fixture.Dirs(t, minDefs, minProjs)
			editDefinition(t, defs, tt.file, tt.change)
			cat, err := catalog.Load(context.Background(), defs, projs)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("Load: %v", err)
				}
				if got := cat.AbsentStructural(); len(got) != 0 {
					t.Errorf("AbsentStructural = %v, want none", got)
				}
				return
			}
			if cat != nil || !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, def.ErrBadManifest) {
				t.Fatalf("Load returned a catalog: %v, err %v; want none, catalog.ErrInvalid and def.ErrBadManifest", cat != nil, err)
			}
			if !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "definitions_dir: "+defs) {
				t.Errorf("error %q does not name %s in definitions_dir %s", err, tt.want, defs)
			}
		})
	}
}

// TestLoad_AdmitsACatalogWithoutAStructuralType: a catalog that lacks record.edge or
// record.document loads, and says which it lacks, so the daemon can log the
// features that degrade.
func TestLoad_AdmitsACatalogWithoutAStructuralType(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		remove []string
		want   []facet.Type
	}{
		{nil, nil},
		{[]string{"record.edge.json"}, []facet.Type{"record.edge"}},
		{[]string{"record.document.json"}, []facet.Type{"record.document"}},
		{[]string{"record.edge.json", "record.document.json"}, []facet.Type{"record.edge", "record.document"}},
	} {
		t.Run(strings.Join(tt.remove, "+"), func(t *testing.T) {
			t.Parallel()
			minDefs, minProjs := fixture.Minimal()
			defs, projs := fixture.Dirs(t, minDefs, minProjs)
			for _, name := range tt.remove {
				if err := os.Remove(filepath.Join(defs, name)); err != nil {
					t.Fatal(err)
				}
			}
			cat, err := catalog.Load(context.Background(), defs, projs)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := cat.AbsentStructural(); !slices.Equal(got, tt.want) {
				t.Errorf("AbsentStructural = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestLoad_StructuralCheckReadsOnlyTheCurrentVersion: history is what was in force
// before, and the consumers read the Definition in force, so a malformed earlier
// version under history/ does not refuse the catalog.
func TestLoad_StructuralCheckReadsOnlyTheCurrentVersion(t *testing.T) {
	t.Parallel()
	minDefs, minProjs := fixture.Minimal()
	defs, projs := fixture.Dirs(t, minDefs, minProjs)
	editDefinition(t, defs, "record.edge.json", func(m map[string]any) { delete(property(m, "kind"), "enum") })
	if err := os.MkdirAll(filepath.Join(defs, "history"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(defs, "record.edge.json"), filepath.Join(defs, "history", "record.edge.v1.json")); err != nil {
		t.Fatal(err)
	}
	minimal, _ := fixture.Minimal()
	body, err := fs.ReadFile(minimal, "record.edge.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defs, "record.edge.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	editDefinition(t, defs, "record.edge.json", func(m map[string]any) {
		m["meta"].(map[string]any)["definition_version"] = 2
	})
	cat, err := catalog.Load(context.Background(), defs, projs)
	if err != nil {
		t.Fatalf("Load with a malformed history version: %v", err)
	}
	if len(cat.History()) != 1 {
		t.Fatalf("history holds %d versions, want the malformed v1", len(cat.History()))
	}
}
