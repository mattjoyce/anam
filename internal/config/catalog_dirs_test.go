package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/ashlar"
)

// definitions_dir and projections_dir: unset means the built-in catalog, the
// environment and the file both set them, a relative path is from the working
// directory whichever layer won, and provenance names the winning origin.
func TestResolvedCatalogDirs(t *testing.T) {
	t.Setenv("ANAM_PERSON_ID", "matt")
	keys := []string{"definitions_dir", "projections_dir"}

	t.Run("unset is empty with the default origin", func(t *testing.T) {
		r, err := config.LoadResolved("")
		if err != nil {
			t.Fatal(err)
		}
		c := r.Value()
		if c.DefinitionsDir != config.DefaultDefinitionsDir || c.ProjectionsDir != config.DefaultProjectionsDir {
			t.Errorf("dirs = %q, %q; want the defaults", c.DefinitionsDir, c.ProjectionsDir)
		}
		for _, k := range keys {
			p, ok := r.Provenance(k)
			if !ok || p.Effective.Kind != ashlar.OriginDefault {
				t.Errorf("%s provenance = %+v, %v; want the default origin", k, p.Effective, ok)
			}
		}
	})

	t.Run("environment sets each, independently", func(t *testing.T) {
		d := t.TempDir()
		t.Setenv("ANAM_DEFINITIONS_DIR", d)
		r, err := config.LoadResolved("")
		if err != nil {
			t.Fatal(err)
		}
		if r.Value().DefinitionsDir != d || r.Value().ProjectionsDir != "" {
			t.Errorf("dirs = %q, %q; want %q and empty", r.Value().DefinitionsDir, r.Value().ProjectionsDir, d)
		}
		p, _ := r.Provenance("definitions_dir")
		if p.Effective.Kind != ashlar.OriginEnv {
			t.Errorf("definitions_dir origin = %q, want env", p.Effective.Kind)
		}
	})

	t.Run("file beats default and keeps its origin after expansion", func(t *testing.T) {
		d := t.TempDir()
		path := writeCfg(t, "person_id: matt\nprojections_dir: "+d+"\n")
		r, err := config.LoadResolved(path)
		if err != nil {
			t.Fatal(err)
		}
		if r.Value().ProjectionsDir != d {
			t.Errorf("projections_dir = %q, want %q", r.Value().ProjectionsDir, d)
		}
		p, _ := r.Provenance("projections_dir")
		if p.Effective.Kind != ashlar.OriginFile || p.Effective.Span.Source == "" {
			t.Errorf("projections_dir origin = %+v, want the file it was written in", p.Effective)
		}
	})

	t.Run("an empty setting selects the built-in catalog and keeps its origin", func(t *testing.T) {
		path := writeCfg(t, "person_id: matt\ndefinitions_dir: \"\"\n")
		r, err := config.LoadResolved(path)
		if err != nil {
			t.Fatal(err)
		}
		if r.Value().DefinitionsDir != "" {
			t.Errorf("definitions_dir = %q, want empty", r.Value().DefinitionsDir)
		}
		p, _ := r.Provenance("definitions_dir")
		if p.Effective.Kind != ashlar.OriginFile {
			t.Errorf("definitions_dir origin = %q, want file", p.Effective.Kind)
		}
	})

	t.Run("relative paths are from the working directory, whoever won", func(t *testing.T) {
		wd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		cfgDir := t.TempDir() // a config beside which a relative path must NOT resolve
		path := filepath.Join(cfgDir, "anam.yaml")
		if err := os.WriteFile(path, []byte("person_id: matt\ndefinitions_dir: cat/defs\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("ANAM_PROJECTIONS_DIR", "cat/projs")
		r, err := config.LoadResolved(path)
		if err != nil {
			t.Fatal(err)
		}
		c := r.Value()
		if want := filepath.Join(wd, "cat/defs"); c.DefinitionsDir != want {
			t.Errorf("definitions_dir = %q, want %q (file-relative)", c.DefinitionsDir, want)
		}
		if want := filepath.Join(wd, "cat/projs"); c.ProjectionsDir != want {
			t.Errorf("projections_dir = %q, want %q (environment-relative)", c.ProjectionsDir, want)
		}
	})
}

// CatalogDirs resolves the two directories by the same layers as a full load, and
// expands them the same way, but does not validate settings it does not return: a
// config a daemon would refuse still names its catalog. Strict decoding still holds.
func TestCatalogDirs(t *testing.T) {
	for _, k := range []string{"ANAM_PERSON_ID", "ANAM_DEFINITIONS_DIR", "ANAM_PROJECTIONS_DIR"} {
		t.Setenv(k, "")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	defs := filepath.Join(dir, "definitions")
	write("timeout: 0s\nbucket_width: fortnight\ndefinitions_dir: " + defs + "\n")
	if _, err := config.Load(path); err == nil {
		t.Fatal("Load accepted a config with no person_id, a zero timeout and an unknown bucket width")
	}
	gotDefs, gotProjs, err := config.CatalogDirs(path)
	if err != nil {
		t.Fatalf("CatalogDirs: %v", err)
	}
	if gotDefs != defs || gotProjs != "" {
		t.Errorf("CatalogDirs = %q, %q; want %q and empty", gotDefs, gotProjs, defs)
	}

	projs := t.TempDir()
	t.Setenv("ANAM_PROJECTIONS_DIR", projs)
	if _, gotProjs, err = config.CatalogDirs(path); err != nil || gotProjs != projs {
		t.Errorf("CatalogDirs projections = %q, %v; want the environment's %q", gotProjs, err, projs)
	}

	t.Chdir(dir)
	write("definitions_dir: relative/defs\n")
	if gotDefs, _, err = config.CatalogDirs(path); err != nil || gotDefs != filepath.Join(dir, "relative", "defs") {
		t.Errorf("CatalogDirs of a relative path = %q, %v; want it from the working directory", gotDefs, err)
	}

	write("definitions_dir: " + defs + "\nno_such_key: 1\n")
	if _, _, err := config.CatalogDirs(path); !errors.Is(err, config.ErrInvalid) {
		t.Errorf("CatalogDirs with an unknown key = %v, want config.ErrInvalid", err)
	}
}
