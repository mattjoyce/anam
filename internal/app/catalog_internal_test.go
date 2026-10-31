package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// A local read refuses a registered Definition the catalog contradicts at the same
// type and version, as the daemon refuses it at start, rather than letting the
// catalog's shadow hide it.
func TestLocalReadRefusesARegisteredDefinitionTheCatalogContradicts(t *testing.T) {
	f := newLedgerFixture(t)
	if _, _, err := cli(t, f.cfgPath, "timeline"); err != nil {
		t.Fatalf("timeline before the contradiction: %v", err)
	}

	m := registerAContradiction(t, f)

	_, _, err := cli(t, f.cfgPath, "timeline")
	if !errors.Is(err, def.ErrBadManifest) {
		t.Fatalf("timeline = %v, want def.ErrBadManifest", err)
	}
	if want := fmt.Sprintf("%s v%d", m.Type, m.Version); !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name %s", err, want)
	}
}

// init checks the catalog before it creates anything: a definitions_dir that does not
// exist refuses the command and leaves no ledger or operational store behind.
func TestInitRefusesAMissingCatalogDirectoryBeforeCreatingFiles(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "no-such-definitions")
	ledgerPath := filepath.Join(dir, "ledger.db")
	dbPath := filepath.Join(dir, "tracking.db")
	cfgPath := filepath.Join(dir, "config.yaml")
	cfg := fmt.Sprintf("person_id: matt\nledger_path: %s\ndb_path: %s\ntimeout: %s\ndefinitions_dir: %s\n",
		ledgerPath, dbPath, testTimeout, missing)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := cli(t, cfgPath, "init")
	if !errors.Is(err, catalog.ErrInvalid) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init = %v, want catalog.ErrInvalid and fs.ErrNotExist", err)
	}
	if !strings.Contains(err.Error(), "definitions_dir") || !strings.Contains(err.Error(), missing) {
		t.Errorf("error %q does not name definitions_dir and %s", err, missing)
	}
	for _, p := range []string{ledgerPath, dbPath} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a refused init left %s (stat: %v)", p, err)
		}
	}
}

// A migration refuses a store holding a registered Definition the catalog contradicts,
// as the daemon and a local read refuse it, and writes nothing.
func TestMigrateRefusesARegisteredDefinitionTheCatalogContradicts(t *testing.T) {
	f := newLedgerFixture(t)
	m := registerAContradiction(t, f)
	newLedger, newStore := filepath.Join(f.dir, "new", "ledger.db"), filepath.Join(f.dir, "new", "tracking.db")

	_, _, err := cli(t, f.cfgPath, "migrate", "--from", f.ledgerPath, "--from-store", filepath.Join(f.dir, "tracking.db"),
		"--to", newLedger, "--to-store", newStore, "--yes")
	if !errors.Is(err, def.ErrBadManifest) {
		t.Fatalf("migrate = %v, want def.ErrBadManifest", err)
	}
	if want := fmt.Sprintf("%s v%d", m.Type, m.Version); !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not name %s", err, want)
	}
	for _, p := range []string{newLedger, newStore} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a refused migration made %s (stat: %v)", p, err)
		}
	}
}

// registerAContradiction registers, in the fixture's operational store, the first
// catalog Definition at its own version with one field added, and returns the
// catalog's manifest it contradicts.
func registerAContradiction(t *testing.T, f ledgerFixture) def.Stored {
	t.Helper()
	manifests, err := def.Manifests()
	if err != nil || len(manifests) == 0 {
		t.Fatalf("no embedded manifests: %v", err)
	}
	m := manifests[0]
	var body map[string]any
	if err := json.Unmarshal(m.Body, &body); err != nil {
		t.Fatal(err)
	}
	body["x_contradiction"] = true
	changed, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	buckets, err := config.Config{BucketWidth: span.Default}.Buckets()
	if err != nil {
		t.Fatal(err)
	}
	db, err := ops.Open(ctx, filepath.Join(f.dir, "tracking.db"), buckets)
	if err != nil {
		t.Fatal(err)
	}
	row := ops.DefinitionRow{Type: string(m.Type), Version: m.Version, Body: string(changed)}
	if err := db.RegisterDefinition(ctx, row, "2026-10-30T00:00:00.000000000Z"); err != nil {
		t.Fatalf("RegisterDefinition: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return m
}

// builtInCatalog is the catalog a test's local command loads with no directories set.
func builtInCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	cat, err := catalog.Load(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	return cat
}
