package daemon

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/catalog"
	"github.com/mattjoyce/anam/internal/catalog/fixture"
	"github.com/mattjoyce/anam/internal/config"
)

// emptyV is the definition set version of no files: SHA-256 over nothing.
const emptyV = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

const (
	msgAdmitted   = "daemon: catalog admitted"
	msgNoCatalog  = "daemon: no catalog configured; set definitions_dir and projections_dir, or register definitions over the API"
	msgStructural = "daemon: structural type not in the catalog"
	msgComponent  = "daemon: catalog component not configured; it is empty"
)

// startLog opens a daemon on cfg with the process logger captured, and returns the
// daemon's startup lines in order. Callers are serial: it replaces the default
// logger.
func startLog(t *testing.T, cfg config.Config) (*Daemon, []map[string]any, error) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	d, err := Open(context.Background(), cfg, Options{})
	if d != nil {
		t.Cleanup(func() { _ = d.closeStores() })
	}
	var lines []map[string]any
	scanner := bufio.NewScanner(&buf)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatal(err)
		}
		if msg, _ := record["msg"].(string); strings.HasPrefix(msg, "daemon: ") {
			lines = append(lines, record)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return d, lines, err
}

// afterAdmitted is the lines from "catalog admitted" on; it fails without one.
func afterAdmitted(t *testing.T, lines []map[string]any) []map[string]any {
	t.Helper()
	for i, l := range lines {
		if l["msg"] == msgAdmitted {
			return lines[i:]
		}
	}
	t.Fatalf("no %q line in %v", msgAdmitted, lines)
	return nil
}

// wantLine fails unless got has the level, message and attributes of want.
func wantLine(t *testing.T, got map[string]any, level, msg string, attrs map[string]any) {
	t.Helper()
	if got["level"] != level || got["msg"] != msg {
		t.Errorf("line = %v %q, want %s %q", got["level"], got["msg"], level, msg)
	}
	for k, v := range attrs {
		if got[k] != v {
			t.Errorf("%q: %s = %v, want %v", msg, k, got[k], v)
		}
	}
}

// minimalConfig is newConfig reading a copy of fixture.Minimal, after change edits
// its definitions directory.
func minimalConfig(t *testing.T, change func(defs string)) config.Config {
	t.Helper()
	cfg := newConfig(t)
	minDefs, minProjs := fixture.Minimal()
	cfg.DefinitionsDir, cfg.ProjectionsDir = fixture.Dirs(t, minDefs, minProjs)
	change(cfg.DefinitionsDir)
	return cfg
}

// The daemon on a config that names no catalog starts on the empty one (card 255,
// R4b): admitted as none with the empty V, a WARN that nothing is configured, then
// one line for each structural type it lacks. The store holds no Definitions.
// Allow-listed in catalogtest: it clears the directories on purpose.
// Serial because it replaces the process default logger.
func TestOpen_NoCatalogStartsEmpty(t *testing.T) {
	cfg := newConfig(t)
	cfg.DefinitionsDir, cfg.ProjectionsDir = "", ""
	d, lines, err := startLog(t, cfg)
	if err != nil {
		t.Fatalf("Open with no catalog: %v", err)
	}
	if d.catalog.SetVersion() != emptyV {
		t.Errorf("set version = %s, want %s", d.catalog.SetVersion(), emptyV)
	}
	lines = afterAdmitted(t, lines)
	if len(lines) != 4 {
		t.Fatalf("logged %d lines from admission, want 4: %v", len(lines), lines)
	}
	wantLine(t, lines[0], "INFO", msgAdmitted, map[string]any{
		"definitions_source": "none", "projections_source": "none", "definition_set_version": emptyV})
	wantLine(t, lines[1], "WARN", msgNoCatalog, nil)
	wantLine(t, lines[2], "INFO", msgStructural, map[string]any{
		"type": "record.edge", "effect": "links cannot be entered or indexed"})
	wantLine(t, lines[3], "INFO", msgStructural, map[string]any{
		"type": "record.document", "effect": "documents cannot be referenced or served"})

	if err := d.closeStores(); err != nil {
		t.Fatal(err)
	}
	rows, err := openStore(t, context.Background(), cfg).Definitions(context.Background())
	if err != nil || len(rows) != 0 {
		t.Errorf("the store holds %d definitions, %v; want none", len(rows), err)
	}

	// One directory left unset is a WARN naming it: that component is empty, and the
	// other keeps its files and, for definitions, its V.
	full := minimalConfig(t, func(string) {})
	fullV := func() string {
		cat, err := catalog.Load(context.Background(), full.DefinitionsDir, full.ProjectionsDir)
		if err != nil {
			t.Fatal(err)
		}
		return cat.SetVersion()
	}()
	label := func(dir string) string {
		if dir == "" {
			return "none"
		}
		return dir
	}
	for _, tt := range []struct {
		unset string
		keep  func(c config.Config) (defs, projs string)
		v     string
	}{
		{"projections_dir", func(c config.Config) (string, string) { return c.DefinitionsDir, "" }, fullV},
		{"definitions_dir", func(c config.Config) (string, string) { return "", c.ProjectionsDir }, emptyV},
	} {
		// A store of its own each time; Minimal's V does not depend on where it is.
		half := minimalConfig(t, func(string) {})
		half.DefinitionsDir, half.ProjectionsDir = tt.keep(half)
		_, lines, err := startLog(t, half)
		if err != nil {
			t.Fatalf("Open with no %s: %v", tt.unset, err)
		}
		lines = afterAdmitted(t, lines)
		want := 2
		if tt.unset == "definitions_dir" {
			want = 4 // and both structural types are absent
		}
		if len(lines) != want {
			t.Fatalf("no %s: logged %d lines from admission, want %d: %v", tt.unset, len(lines), want, lines)
		}
		wantLine(t, lines[0], "INFO", msgAdmitted, map[string]any{
			"definitions_source": label(half.DefinitionsDir), "projections_source": label(half.ProjectionsDir), "definition_set_version": tt.v})
		wantLine(t, lines[1], "WARN", msgComponent, map[string]any{"unset": tt.unset})
	}
}

// A catalog without record.edge is admitted, and the daemon says which feature
// degrades. It is configured, so there is no WARN.
// Serial because it replaces the process default logger.
func TestOpen_AbsentEdgeDegrades(t *testing.T) {
	cfg := minimalConfig(t, func(defs string) {
		if err := os.Remove(filepath.Join(defs, "record.edge.json")); err != nil {
			t.Fatal(err)
		}
	})
	_, lines, err := startLog(t, cfg)
	if err != nil {
		t.Fatalf("Open without record.edge: %v", err)
	}
	lines = afterAdmitted(t, lines)
	if len(lines) != 2 {
		t.Fatalf("logged %d lines from admission, want 2: %v", len(lines), lines)
	}
	wantLine(t, lines[0], "INFO", msgAdmitted, map[string]any{
		"definitions_source": cfg.DefinitionsDir, "projections_source": cfg.ProjectionsDir})
	wantLine(t, lines[1], "INFO", msgStructural, map[string]any{
		"type": "record.edge", "effect": "links cannot be entered or indexed"})
}

// A configured catalog that holds both structural types logs only its admission:
// no WARN, no structural line, and no drift report, which went with the embedded
// catalog. Serial because it replaces the process default logger.
func TestOpen_ACompleteCatalogLogsOnlyItsAdmission(t *testing.T) {
	for name, cfg := range map[string]config.Config{
		"minimal":   minimalConfig(t, func(string) {}),
		"mattjoyce": newConfig(t),
	} {
		t.Run(name, func(t *testing.T) {
			_, lines, err := startLog(t, cfg)
			if err != nil {
				t.Fatal(err)
			}
			lines = afterAdmitted(t, lines)
			if len(lines) != 1 {
				t.Fatalf("logged %d lines from admission, want 1: %v", len(lines), lines)
			}
			wantLine(t, lines[0], "INFO", msgAdmitted, map[string]any{
				"definitions_source": cfg.DefinitionsDir, "projections_source": cfg.ProjectionsDir})
		})
	}
}

// A structural type in a shape the engine cannot read is refused at start, naming
// the type, its version and the pointer, before either store is opened: the ledger
// and the operational store are byte for byte as the last good start left them, and
// nothing is persisted. Sol's review of #37 found a record.document whose sha256 was
// a number persisted and a reference admitted with no bytes behind it.
// Serial because it replaces the process default logger.
func TestOpen_RefusesAMalformedStructuralTypeWithTheStoreUntouched(t *testing.T) {
	for _, tt := range []struct {
		name, file, from, to, want string
	}{
		{"edge kind without its enum", "record.edge.json", `"enum": [
          "about",
          "interpreted_from"
        ]`, `"minLength": 1`, "record.edge v1 /kind"},
		{"edge kind nullable", "record.edge.json", `"kind": {
        "type": "string",
        "enum": [
          "about",
          "interpreted_from"
        ]`, `"kind": {
        "type": ["string", "null"],
        "enum": [
          "about",
          null
        ]`, "record.edge v1 /kind"},
		{"document sha256 a number", "record.document.json", `"sha256": {
        "type": "string",
        "minLength": 1,
        "pattern": "^[0-9a-f]{64}$"
      }`, `"sha256": {
        "type": "number"
      }`, "record.document v1 /sha256"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalConfig(t, func(string) {})
			d, _, err := startLog(t, cfg)
			if err != nil {
				t.Fatal(err)
			}
			if err := d.closeStores(); err != nil {
				t.Fatal(err)
			}
			before := storeFiles(t, cfg)

			path := filepath.Join(cfg.DefinitionsDir, tt.file)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			broken := bytes.Replace(raw, []byte(tt.from), []byte(tt.to), 1)
			if bytes.Equal(broken, raw) {
				t.Fatalf("the fixture's %s does not hold the text to change", tt.file)
			}
			if err := os.WriteFile(path, broken, 0o600); err != nil {
				t.Fatal(err)
			}

			d, lines, err := startLog(t, cfg)
			if d != nil || !errors.Is(err, catalog.ErrInvalid) {
				t.Fatalf("Open returned a daemon: %v, err %v; want a refusal wrapping catalog.ErrInvalid", d != nil, err)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("refusal %q does not name %s", err, tt.want)
			}
			for _, l := range lines {
				if l["msg"] == msgAdmitted {
					t.Errorf("a refused catalog was admitted: %v", l)
				}
			}
			after := storeFiles(t, cfg)
			if len(after) != len(before) {
				t.Fatalf("store files changed from %v to %v", before, after)
			}
			for name, sum := range before {
				if after[name] != sum {
					t.Errorf("%s changed under a refused start", name)
				}
			}
		})
	}
}

// storeFiles is the SHA-256 of every file beside the ledger and the operational
// store, by name, so a test sees a write, a new WAL or a deleted file.
func storeFiles(t *testing.T, cfg config.Config) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	for _, dir := range []string{filepath.Dir(cfg.LedgerPath), filepath.Dir(cfg.DBPath)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			p := filepath.Join(dir, e.Name())
			raw, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			out[p] = sha256.Sum256(raw)
		}
	}
	return out
}
