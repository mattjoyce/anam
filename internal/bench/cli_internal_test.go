package bench

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/catalog"
)

// The bench reads the catalog the anam config chooses, as anam does, and says which;
// a directory that is missing refuses the invocation rather than falling back to the
// built-in catalog. Not parallel: it clears the environment the config would read.
func TestLoadCatalog_FollowsTheConfig(t *testing.T) {
	for _, k := range []string{"ANAM_CONFIG", "ANAM_DEFINITIONS_DIR", "ANAM_PROJECTIONS_DIR"} {
		t.Setenv(k, "")
	}
	dir := t.TempDir()
	defs := filepath.Join(dir, "definitions")
	if err := os.Mkdir(defs, 0o750); err != nil {
		t.Fatal(err)
	}
	writeConfig := func(definitionsDir string) string {
		t.Helper()
		path := filepath.Join(dir, "config.yaml")
		if err := os.WriteFile(path, []byte(fmt.Sprintf("definitions_dir: %s\n", definitionsDir)), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	run := func(cfgPath string) (*catalog.Catalog, string, error) {
		t.Helper()
		var stderr bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		cmd.SetErr(&stderr)
		cat, err := loadCatalog(cmd, cfgPath, Person)
		return cat, stderr.String(), err
	}

	cat, stderr, err := run(writeConfig(defs))
	if err != nil {
		t.Fatalf("loadCatalog: %v", err)
	}
	if cat.DefinitionsSource() != defs || cat.ProjectionsSource() != "embedded" {
		t.Errorf("sources = %s, %s; want %s, embedded", cat.DefinitionsSource(), cat.ProjectionsSource(), defs)
	}
	if !strings.Contains(stderr, "definitions "+defs) {
		t.Errorf("the bench does not say which catalog it read:\n%s", stderr)
	}

	missing := filepath.Join(dir, "no-such-definitions")
	_, _, err = run(writeConfig(missing))
	if !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("loadCatalog of a missing directory = %v, want catalog.ErrInvalid and fs.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not name %s", err, missing)
	}
}
