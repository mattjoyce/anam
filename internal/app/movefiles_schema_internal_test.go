package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/store/ops"
	"github.com/mattjoyce/anam/internal/store/ops/opstest"
)

// Card 260: anam move-artifacts --registered-from over a store the schema-9 daemon
// wrote (opstest.Schema9). It covers the report, the carry into a fresh schema-10 store,
// the refusal of a second carry, and every receipt verifying from the new store.

// schema9Move is the fixture, a config over its ledger and a fresh store on the catalog
// the schema-9 daemon ran on, and an empty artifact folder.
type schema9Move struct {
	opstest.Schema9Paths
	cfg, store, folder string
}

func newSchema9Move(t *testing.T) schema9Move {
	t.Helper()
	dir := t.TempDir()
	p, err := opstest.Schema9(context.Background(), filepath.Join(dir, "old"))
	if err != nil {
		t.Fatal(err)
	}
	m := schema9Move{Schema9Paths: p, store: filepath.Join(dir, "new", "tracking.db"), folder: filepath.Join(dir, "folder")}
	if err := os.MkdirAll(m.folder, 0o750); err != nil {
		t.Fatal(err)
	}
	m.cfg = filepath.Join(dir, "anam.yaml")
	writeDoc(t, m.cfg, fmt.Sprintf("person_id: matt\nledger_path: %s\ndb_path: %s\ntimeout: %s\ndefinitions_dir: %s\nprojections_dir: %s\n",
		p.Ledger, m.store, testTimeout, p.Definitions, p.Projections))
	return m
}

// carriedTables is each table the carry writes, by its schema-10 name, and the name the
// schema-9 file gives it.
var carriedTables = map[string]string{
	"registered_definition": "registered_definition", "registered_projection": "registered_projection",
	"template_body": "inline_projection", "admission_hold": "admission_hold", "receipt": "receipt",
	"served_value": "served_value", "saved_selection": "saved_selection",
	"saved_selection_code": "saved_selection_code", "definition": "definition",
}

func tableRows(t *testing.T, path, table string) []string {
	t.Helper()
	r, err := opstest.Rows(context.Background(), path, table)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestMoveArtifacts_CarriesASchema9StoreIntoSchema10(t *testing.T) {
	t.Parallel()
	m := newSchema9Move(t)
	before := sumOf(t, m.Store)
	counts := "2 registered Definitions, 2 registered projection templates, 4 template bodies, 2 admission holds, " +
		"6 receipts, 3 served values, 3 saved selections, 1 selection codes and 8 provisioned Definitions"

	out, _, err := cli(t, m.cfg, "move-artifacts", "--from", m.folder, "--registered-from", m.Store)
	if err != nil {
		t.Fatalf("report: %v", err)
	}
	for _, want := range []string{"schema version 9", "would copy " + counts, "facet_index 6", "nothing written"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report lacks %q:\n%s", want, out)
		}
	}
	for table := range carriedTables {
		if got := tableRows(t, m.store, table); len(got) != 0 {
			t.Fatalf("the report wrote %d %s rows", len(got), table)
		}
	}

	out, _, err = cli(t, m.cfg, "move-artifacts", "--from", m.folder, "--registered-from", m.Store, "--yes")
	if err != nil {
		t.Fatalf("carry: %v", err)
	}
	if !strings.Contains(out, "copied "+counts) {
		t.Errorf("the carry does not say what it copied:\n%s", out)
	}
	for now, was := range carriedTables {
		if got, want := tableRows(t, m.store, now), tableRows(t, m.Store, was); !slices.Equal(got, want) {
			t.Errorf("%s after the carry:\n%v\n%s before it:\n%v", now, got, was, want)
		}
	}

	// Again with --registered-from. It is refused, and the store and the old file are as they were.
	carriedRows := map[string][]string{}
	for now := range carriedTables {
		carriedRows[now] = tableRows(t, m.store, now)
	}
	_, _, err = cli(t, m.cfg, "move-artifacts", "--from", m.folder, "--registered-from", m.Store, "--yes")
	if err == nil || !strings.Contains(err.Error(), "not fresh") || !strings.Contains(err.Error(), "without --registered-from") {
		t.Fatalf("a second carry = %v, want the not-fresh refusal saying how to go on", err)
	}
	for now, want := range carriedRows {
		if got := tableRows(t, m.store, now); !slices.Equal(got, want) {
			t.Errorf("the refused second carry changed %s", now)
		}
	}
	if sumOf(t, m.Store) != before {
		t.Error("the carry changed the old file")
	}
	// Without it, the folder step runs alone, which is how a run that failed after its
	// carry is finished.
	if _, _, err := cli(t, m.cfg, "move-artifacts", "--from", m.folder, "--yes"); err != nil {
		t.Errorf("the folder step after the carry: %v", err)
	}

	// Every receipt verifies from the new store, against the old ledger.
	raw, err := opstest.Schema9File("expect.json")
	if err != nil {
		t.Fatal(err)
	}
	var exp struct {
		Receipts map[string]json.RawMessage `json:"receipts"`
	}
	if err := json.Unmarshal(raw, &exp); err != nil {
		t.Fatal(err)
	}
	for hash := range exp.Receipts {
		verified, _, err := cli(t, m.cfg, "receipt", "verify", hash)
		if err != nil || !strings.Contains(verified, "OK   reproduced") || !strings.Contains(verified, "OK   intact") {
			t.Errorf("receipt %s after the carry: %v\n%s", hash, err, verified)
		}
	}
}

// The command refuses to carry a store into itself, and a source it cannot read whole
// is refused before the store it writes into is made.
func TestMoveArtifacts_RefusesASourceItCannotCarry(t *testing.T) {
	t.Parallel()
	m := newSchema9Move(t)
	same := filepath.Join(filepath.Dir(m.cfg), "same.yaml")
	writeDoc(t, same, fmt.Sprintf("person_id: matt\nledger_path: %s\ndb_path: %s\ntimeout: %s\ndefinitions_dir: %s\nprojections_dir: %s\n",
		m.Ledger, m.Store, testTimeout, m.Definitions, m.Projections))
	if _, _, err := cli(t, same, "move-artifacts", "--from", m.folder, "--registered-from", m.Store, "--yes"); err == nil ||
		!strings.Contains(err.Error(), "is the store this command writes into") {
		t.Fatalf("a carry of a store into itself = %v", err)
	}

	ctx := context.Background()
	db, err := ops.OpenReadOnly(ctx, m.Store)
	if err == nil {
		_ = db.Close()
		t.Fatal("OpenReadOnly opened the schema-9 file")
	}
	edited := filepath.Join(filepath.Dir(m.cfg), "edited")
	p, err := opstest.Schema9(ctx, edited)
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile(p.Store) //nolint:gosec // a path under the test's own directory
	if err != nil {
		t.Fatal(err)
	}
	// A file that is not SQLite at all.
	if err := os.WriteFile(p.Store, src[:100], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := cli(t, m.cfg, "move-artifacts", "--from", m.folder, "--registered-from", p.Store, "--yes"); err == nil ||
		!strings.Contains(err.Error(), "nothing was written") {
		t.Fatalf("a carry from a damaged file = %v", err)
	}
	if _, err := os.Stat(m.store); err == nil {
		t.Error("a refused carry made the store it would have written into")
	}
}
