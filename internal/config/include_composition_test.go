package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mattjoyce/ashlar"
	"github.com/mattjoyce/ashlar/yamlconfig"
)

// Exercise container composition before strict Config decoding. Runtime has no
// array-valued setting yet, so these values must never become accepted config keys.
func TestIncludeContainerComposition(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, child string
		want        []string
	}{
		{"array replacement", "section:\n  items: [later]", []string{"later"}},
		{"empty array clears", "section:\n  items: []", []string{}},
		{"top level null omission", "section: null", []string{"earlier", "earlier"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fragments := compositionFiles(t, "section:\n  items: [earlier, earlier]\n  kept: root\ninclude: child.yaml", tt.child)
			resolved, err := yamlconfig.ResolveFragments(context.Background(), fragments, yamlconfig.ResolveOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Section struct {
					Items []string `yaml:"items"`
					Kept  string   `yaml:"kept"`
				} `yaml:"section"`
			}
			if err := resolved.DecodeStrict(&decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded.Section.Items, tt.want) || decoded.Section.Kept != "root" {
				t.Fatalf("composed section = %+v", decoded.Section)
			}
			items := resolved.Root().At("/section/items")
			wantSource := "child.yaml"
			if tt.child == "section: null" {
				wantSource = "config.yaml"
			}
			if items.Provenance().Effective.Span.Source != wantSource {
				t.Fatalf("array origin = %+v", items.Provenance())
			}
		})
	}
	for _, child := range []string{"section: []", "section:\n  items: {}", "section:\n  items: null"} {
		t.Run("conflict "+child, func(t *testing.T) {
			t.Parallel()
			fragments := compositionFiles(t, "section:\n  items: [earlier]\ninclude: child.yaml", child)
			if _, err := yamlconfig.ResolveFragments(context.Background(), fragments, yamlconfig.ResolveOptions{}); !errors.Is(err, ashlar.ErrComposition) {
				t.Fatalf("kind conflict error = %v", err)
			}
		})
	}
}

func TestIncludeNestedValidationProvenance(t *testing.T) {
	t.Parallel()
	fragments := compositionFiles(t, "include: child.yaml", "include: null\nsection:\n  items:\n    - value: invalid\n")
	resolved, err := yamlconfig.ResolveFragments(context.Background(), fragments, yamlconfig.ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := Resolved{root: resolved.Root()}
	pointer := ashlar.Pointer("/section/items/0/value")
	err = r.validationError(ashlar.NewError(ErrInvalid, pointer, ashlar.Origin{}, "invalid nested value", nil))
	var diagnostic *ashlar.Error
	if !errors.As(err, &diagnostic) {
		t.Fatal(err)
	}
	if diagnostic.Pointer != pointer || diagnostic.Origin.Span.Source != "child.yaml" || diagnostic.Origin.Span.Start.Line != 4 || len(diagnostic.Origin.Via()) != 1 {
		t.Fatalf("nested diagnostic = %v", err)
	}
	if _, ok := r.ProvenanceAt("/section/absent"); ok {
		t.Fatal("absent pointer has provenance")
	}
}

func compositionFiles(t *testing.T, root, child string) []yamlconfig.Fragment {
	t.Helper()
	dir := t.TempDir()
	for name, body := range map[string]string{"config.yaml": root, "child.yaml": child} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fragments, err := readConfigFragments(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return fragments
}
