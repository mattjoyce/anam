package catalog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
)

func TestDriftFrom(t *testing.T) {
	t.Parallel()
	for _, tt := range driftCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			embedded := loadEmbedded(t)
			defs, projs := t.TempDir(), t.TempDir()
			copyEmbeddedDefs(t, defs)
			copyEmbeddedProjections(t, projs)
			if tt.change != nil {
				tt.change(t, defs, projs)
			}
			cat, err := catalog.Load(context.Background(), defs, projs)
			if err != nil {
				t.Fatal(err)
			}
			got := cat.DriftFrom(embedded)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("DriftFrom = %+v, want %+v", got, tt.want)
			}
			if got.None() != (tt.change == nil) {
				t.Errorf("None = %v", got.None())
			}
			reverse := embedded.DriftFrom(cat)
			wantReverse := catalog.Drift{
				DefinitionsOnlyHere: tt.want.DefinitionsOnlyThere, DefinitionsOnlyThere: tt.want.DefinitionsOnlyHere, DefinitionsDiffer: tt.want.DefinitionsDiffer,
				ProjectionsOnlyHere: tt.want.ProjectionsOnlyThere, ProjectionsOnlyThere: tt.want.ProjectionsOnlyHere, ProjectionsDiffer: tt.want.ProjectionsDiffer,
				SetVersionDiffers: tt.want.SetVersionDiffers,
			}
			if !reflect.DeepEqual(reverse, wantReverse) {
				t.Errorf("reverse = %+v, want %+v", reverse, wantReverse)
			}
			// Removing sources after loading cannot change a frozen comparison.
			if err := os.RemoveAll(defs); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(projs); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(cat.DriftFrom(embedded), got) {
				t.Fatal("drift changed after source removal")
			}
		})
	}
}

type driftCase struct {
	name   string
	change func(*testing.T, string, string)
	want   catalog.Drift
}

func driftCases() []driftCase {
	return []driftCase{
		{name: "identical copy"},
		{name: "appended byte", change: func(t *testing.T, defs, _ string) {
			rewriteCatalogFile(t, defs, "health.weight.json", func(raw []byte) []byte { return append(raw, '\n') })
		}, want: catalog.Drift{DefinitionsDiffer: []string{"health.weight.json"}, SetVersionDiffers: true}},
		{name: "reformatted definition", change: func(t *testing.T, defs, _ string) {
			rewriteCatalogFile(t, defs, "health.weight.json", func(raw []byte) []byte {
				var buf bytes.Buffer
				if err := json.Compact(&buf, raw); err != nil {
					t.Fatal(err)
				}
				if bytes.Equal(raw, buf.Bytes()) {
					t.Fatal("fixture is already compact")
				}
				return buf.Bytes()
			})
		}, want: catalog.Drift{DefinitionsDiffer: []string{"health.weight.json"}, SetVersionDiffers: true}},
		{name: "removed history", change: func(t *testing.T, defs, _ string) {
			if err := os.Remove(filepath.Join(defs, "history/health.weight.v2.json")); err != nil {
				t.Fatal(err)
			}
		}, want: catalog.Drift{DefinitionsOnlyThere: []string{"history/health.weight.v2.json"}}},
		{name: "added projections sorted", change: func(t *testing.T, _, projs string) {
			raw, err := os.ReadFile(filepath.Join(projs, "bp.json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"zz-extra", "aa-extra"} {
				body := bytes.Replace(raw, []byte(`"name": "bp"`), []byte(`"name": "`+name+`"`), 1)
				if err := os.WriteFile(filepath.Join(projs, name+".json"), body, 0o600); err != nil {
					t.Fatal(err)
				}
			}
		}, want: catalog.Drift{ProjectionsOnlyHere: []string{"aa-extra.json", "zz-extra.json"}}},
		{name: "changed projections and history sorted", change: func(t *testing.T, defs, projs string) {
			for _, name := range []string{"meds.json", "bp.json", "history/meds.v1.json"} {
				rewriteCatalogFile(t, projs, name, func(raw []byte) []byte { return append(raw, '\n') })
			}
			rewriteCatalogFile(t, defs, "history/health.weight.v2.json", func(raw []byte) []byte { return append(raw, '\n') })
		}, want: catalog.Drift{DefinitionsDiffer: []string{"history/health.weight.v2.json"}, ProjectionsDiffer: []string{"bp.json", "history/meds.v1.json", "meds.json"}}},
	}
}

func rewriteCatalogFile(t *testing.T, dir, name string, change func([]byte) []byte) {
	t.Helper()
	path := filepath.Join(dir, name)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, change(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}
