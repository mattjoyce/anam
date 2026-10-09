package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/capture/documents"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/span"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Card 29: the default library is a documents import that needs no declaration. It
// is added when the folder exists, skipped when it does not, and never doubles a
// declared documents import.
func TestWithLibrary_ImplicitWhenTheFolderExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	cfg := config.Config{LibraryDir: filepath.Join(dir, "documents")}
	declared := []config.Import{{Source: "records", Path: filepath.Join(dir, "records")}}

	if got := withLibrary(declared, cfg); len(got) != 1 {
		t.Fatalf("absent library added an import: %v", got)
	}
	if err := os.MkdirAll(cfg.LibraryDir, 0o750); err != nil {
		t.Fatal(err)
	}
	got := withLibrary(declared, cfg)
	if len(got) != 2 || got[1].Source != documents.Source || got[1].Path != cfg.LibraryDir {
		t.Fatalf("present library not added: %v", got)
	}
	explicit := append(declared, config.Import{Source: documents.Source, Path: "/elsewhere"})
	if got := withLibrary(explicit, cfg); len(got) != 2 || got[1].Path != "/elsewhere" {
		t.Fatalf("declared documents import not left alone: %v", got)
	}
}

// Card 14, wired: the first ingest stores the bundled Definitions; a later build
// whose keying differs is refused before any source is read, and the stored set is
// left as it was.
func TestGuardDefinitions_StoresThenRefusesAChangedKeying(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	cfg := config.Config{
		DBPath: filepath.Join(t.TempDir(), "tracking.db"), BucketWidth: span.Default, Timeout: 5 * time.Second,
	}
	reg, err := def.Bundled()
	if err != nil {
		t.Fatalf("def.Bundled: %v", err)
	}
	if err := guardDefinitions(ctx, cfg, reg); err != nil {
		t.Fatalf("first guard: %v", err)
	}
	buckets, _ := cfg.Buckets()
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		t.Fatalf("ops.Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	rows, err := db.Definitions(ctx)
	if err != nil || len(rows) != len(reg.All()) {
		t.Fatalf("stored %d definitions, %v; want %d", len(rows), err, len(reg.All()))
	}

	// Pretend the store last saw blood pressure keyed another way.
	var bp ops.DefinitionRow
	for _, r := range rows {
		if r.Type == "health.blood-pressure" {
			bp = r
		}
	}
	bp.Body = strings.Replace(bp.Body, `"identity_key": ["/date_excel", "/reading"]`, `"identity_key": ["/date_excel"]`, 1)
	if !strings.Contains(bp.Body, `"identity_key": ["/date_excel"]`) {
		t.Fatalf("fixture did not change the stored body")
	}
	if err := db.PutDefinitions(ctx, []ops.DefinitionRow{bp}); err != nil {
		t.Fatalf("PutDefinitions: %v", err)
	}
	if err := guardDefinitions(ctx, cfg, reg); !errors.Is(err, def.ErrKeyed) {
		t.Fatalf("guard against a differently keyed store = %v, want ErrKeyed", err)
	}
}

// Ingest against a path with no ledger fails naming `anam init`, and leaves nothing
// behind: not an operational store either, which would look like the start of a
// record where there is none (card 46, the ThinkPad after the move).
func TestIngest_WithNoLedgerCreatesNothing(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	records := filepath.Join(dir, "records")
	if err := os.MkdirAll(records, 0o750); err != nil {
		t.Fatal(err)
	}
	imports := filepath.Join(dir, "imports.yaml")
	if err := os.WriteFile(imports, []byte("imports:\n  - source: records\n    path: "+records+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfgPath := filepath.Join(dir, "anam.yaml")
	cfg := "person_id: matt\nledger_path: " + filepath.Join(data, "ledger.db") +
		"\ndb_path: " + filepath.Join(data, "tracking.db") + "\nartifact_dir: " + filepath.Join(data, "artifacts") + "\n"
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err := cli(t, cfgPath, "ingest", "--imports", imports)
	if err == nil || !strings.Contains(err.Error(), "anam init") {
		t.Fatalf("ingest with no ledger: %v, want an error naming anam init", err)
	}
	if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ingest with no ledger left %s behind (%v)", data, err)
	}
}
