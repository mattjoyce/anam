package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
)

func writeImports(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "imports.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadImports_ExpandsDeclaredPaths(t *testing.T) {
	t.Parallel()
	path := writeImports(t, `
imports:
  - source: health-db
    path: ~/admin/data/healthdata.db
  - source: bp-csv
    path: $HOME/exports/bp.csv
`)
	imports, err := config.LoadImports(path)
	if err != nil {
		t.Fatalf("LoadImports: %v", err)
	}
	if len(imports) != 2 {
		t.Fatalf("loaded %d imports, want 2", len(imports))
	}
	home, _ := os.UserHomeDir()
	if imports[0].Source != "health-db" || imports[0].Path != filepath.Join(home, "admin/data/healthdata.db") {
		t.Errorf("import 0 = %+v — tilde not expanded", imports[0])
	}
	if imports[1].Path != filepath.Join(home, "exports/bp.csv") {
		t.Errorf("import 1 = %+v — $HOME not expanded", imports[1])
	}
}

func TestLoadImports_Refusals(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body, wantMsg string
	}{
		{"unknown key", "imports:\n  - source: bp-csv\n    path: /x\n    sorce: typo\n", "invalid imports"},
		{"no imports", "imports: []\n", "declares no imports"},
		{"missing source", "imports:\n  - path: /x\n", "has no source"},
		{"missing path", "imports:\n  - source: bp-csv\n", "has no path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := config.LoadImports(writeImports(t, tc.body))
			if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("got %v, want error containing %q", err, tc.wantMsg)
			}
		})
	}
}

func TestResolveImports_ExplicitMustExist(t *testing.T) {
	t.Parallel()
	if _, _, err := config.ResolveImports(filepath.Join(t.TempDir(), "absent.yaml")); !errors.Is(err, config.ErrNotFound) {
		t.Errorf("explicit absent imports = %v, want ErrNotFound", err)
	}

	path := writeImports(t, "imports:\n  - source: bp-csv\n    path: /x\n")
	got, origin, err := config.ResolveImports(path)
	if err != nil || got != path || origin != config.OriginFlag {
		t.Errorf("ResolveImports(explicit) = %q %q %v", got, origin, err)
	}
}
