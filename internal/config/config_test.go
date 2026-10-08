package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/span"
)

// TestLoadWithoutPersonIDFails guards the person pin. person_id has no default:
// defaulting it would pin a ledger to a person nobody chose.
func TestLoadWithoutPersonIDFails(t *testing.T) {
	t.Setenv("ANAM_PERSON_ID", "") // hermetic: ignore the developer's own environment

	_, err := config.Load("")
	if err == nil {
		t.Fatal("Load() = nil error with no person_id, want error")
	}
	if !errors.Is(err, config.ErrInvalid) {
		t.Errorf("Load() error = %v, want it to wrap ErrInvalid", err)
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ANAM_PERSON_ID", "matt")
	t.Setenv("XDG_DATA_HOME", "/data")

	got, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config.Config{
		PersonID:    "matt",
		LedgerPath:  "/data/anam/ledger.db",
		DBPath:      "/data/anam/tracking.db",
		ArtifactDir: "/data/anam/artifacts",
		LibraryDir:  "/data/anam/documents",
		Zone:        "Local",
		Listen:      config.DefaultListen,
		Timeout:     5 * time.Second,
		BucketWidth: span.Default,
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestDefaultDataPaths(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	if got, want := config.DefaultLedgerPath(), "/data/anam/ledger.db"; got != want {
		t.Errorf("DefaultLedgerPath() = %q, want %q (XDG_DATA_HOME honoured)", got, want)
	}
	if got, want := config.DefaultDBPath(), "/data/anam/tracking.db"; got != want {
		t.Errorf("DefaultDBPath() = %q, want %q (XDG_DATA_HOME honoured)", got, want)
	}
	if got, want := config.DefaultArtifactDir(), "/data/anam/artifacts"; got != want {
		t.Errorf("DefaultArtifactDir() = %q, want %q", got, want)
	}

	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/tester")
	if got, want := config.DefaultDBPath(), "/home/tester/.local/share/anam/tracking.db"; got != want {
		t.Errorf("DefaultDBPath() = %q, want %q (XDG fallback)", got, want)
	}
}

// TestPersonIDComesFromEnvWithoutADefault pins the reason person_id is bound rather
// than left alone: a key with no default and no config entry is unknown to viper, so
// Unmarshal would silently ignore ANAM_PERSON_ID.
func TestPersonIDComesFromEnvWithoutADefault(t *testing.T) {
	t.Setenv("ANAM_PERSON_ID", "someone-else")

	got, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.PersonID != "someone-else" {
		t.Errorf("PersonID = %q, want the environment value %q", got.PersonID, "someone-else")
	}
}

func TestLoadFromFile(t *testing.T) {
	path := writeConfig(t, `
person_id: matt
ledger_path: /srv/anam/ledger.db
db_path: /srv/anam/tracking.db
artifact_dir: /srv/anam/artifacts
timeout: 12s
bucket_width: weekly
`)

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config.Config{
		PersonID:    "matt",
		LedgerPath:  "/srv/anam/ledger.db",
		DBPath:      "/srv/anam/tracking.db",
		ArtifactDir: "/srv/anam/artifacts",
		LibraryDir:  "/srv/anam/documents",
		Zone:        "Local",
		Listen:      config.DefaultListen,
		Timeout:     12 * time.Second,
		BucketWidth: span.Weekly,
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFileLeavesUnsetKeysAtDefaults(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	path := writeConfig(t, "person_id: matt\nledger_path: /srv/partial/ledger.db\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.LedgerPath != "/srv/partial/ledger.db" {
		t.Errorf("LedgerPath = %q, want %q", got.LedgerPath, "/srv/partial/ledger.db")
	}
	if got.DBPath != "/data/anam/tracking.db" {
		t.Errorf("DBPath = %q, want the default", got.DBPath)
	}
	if got.PersonID != "matt" {
		t.Errorf("PersonID = %q, want the file value %q", got.PersonID, "matt")
	}
	if got.Timeout != 5*time.Second {
		t.Errorf("Timeout = %s, want the default 5s", got.Timeout)
	}
}

func TestEnvOverridesFileAndDefaults(t *testing.T) {
	path := writeConfig(t, "person_id: from-file\nledger_path: /from-file/ledger.db\n")

	t.Setenv("ANAM_LEDGER_PATH", "/from-env/ledger.db")
	t.Setenv("ANAM_TIMEOUT", "30s")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.LedgerPath != "/from-env/ledger.db" {
		t.Errorf("LedgerPath = %q, want the environment to override the file", got.LedgerPath)
	}
	if got.Timeout != 30*time.Second {
		t.Errorf("Timeout = %s, want the environment to override the default", got.Timeout)
	}
	if got.PersonID != "from-file" {
		t.Errorf("PersonID = %q, want the file value where the environment is silent", got.PersonID)
	}
}

// TestLoadRejectsUnknownKey is the fail-fast guard: a mistyped key must not be
// silently ignored, leaving Anam writing to the default ledger while the operator
// believes it is writing to theirs. The retired DynamoDB keys are refused by the same
// rule, so a config written for the old ledger fails at startup and names the key.
func TestLoadRejectsUnknownKey(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"typo'd key", "ledger_pth: /x/ledger.db\n"},
		{"key from a future version", "seal_point: true\n"},
		{"camelCase instead of snake_case", "ledgerPath: /x/ledger.db\n"},
		{"retired DynamoDB endpoint", "ddb_endpoint: http://x:8000\n"},
		{"retired DynamoDB table", "table_name: anam\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.body)

			_, err := config.Load(path)
			if err == nil {
				t.Fatalf("Load() = nil error for %q, want an error naming the unknown key", tt.body)
			}
			if !errors.Is(err, config.ErrInvalid) {
				t.Errorf("Load() error = %v, want it to wrap ErrInvalid", err)
			}
		})
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := config.Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err == nil {
		t.Fatal("Load() = nil error for a missing file, want error")
	}
}

func TestLoadMalformedFile(t *testing.T) {
	path := writeConfig(t, "person_id: [unclosed\n")

	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() = nil error for malformed YAML, want error")
	}
}

func TestLoadRejectsInvalidConfig(t *testing.T) {
	path := writeConfig(t, "person_id: \"\"\n")

	_, err := config.Load(path)
	if err == nil {
		t.Fatal("Load() = nil error for an empty person_id, want error")
	}
	if !errors.Is(err, config.ErrInvalid) {
		t.Errorf("Load() error = %v, want it to wrap ErrInvalid", err)
	}
}

func TestValidate(t *testing.T) {
	t.Parallel()

	valid := config.Config{
		PersonID:    "matt",
		LedgerPath:  "/srv/ledger.db",
		DBPath:      "/srv/tracking.db",
		ArtifactDir: "/srv/artifacts",
		LibraryDir:  "/srv/documents",
		Zone:        "Australia/Sydney",
		Timeout:     5 * time.Second,
		BucketWidth: span.Default,
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr bool
	}{
		{"valid", func(*config.Config) {}, false},
		{"missing person_id", func(c *config.Config) { c.PersonID = "" }, true},
		{"missing ledger_path", func(c *config.Config) { c.LedgerPath = "" }, true},
		{"ledger and tracking in one file", func(c *config.Config) { c.LedgerPath = c.DBPath }, true},
		{"missing db_path", func(c *config.Config) { c.DBPath = "" }, true},
		{"missing artifact_dir", func(c *config.Config) { c.ArtifactDir = "" }, true},
		{"missing library_dir", func(c *config.Config) { c.LibraryDir = "" }, true},
		{"unknown zone", func(c *config.Config) { c.Zone = "Mars/Olympus" }, true},
		{"utc zone", func(c *config.Config) { c.Zone = "UTC" }, false},
		{"zero timeout", func(c *config.Config) { c.Timeout = 0 }, true},
		{"negative timeout", func(c *config.Config) { c.Timeout = -time.Second }, true},
		{"weekly bucket_width", func(c *config.Config) { c.BucketWidth = span.Weekly }, false},
		{"unknown bucket_width", func(c *config.Config) { c.BucketWidth = "fortnightly" }, true},
		{"empty bucket_width", func(c *config.Config) { c.BucketWidth = "" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := valid
			tt.mutate(&cfg)
			err := cfg.Validate()

			if tt.wantErr {
				if err == nil {
					t.Fatal("Validate() = nil, want error")
				}
				if !errors.Is(err, config.ErrInvalid) {
					t.Errorf("Validate() error = %v, want it to wrap ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Errorf("Validate() = %v, want nil", err)
			}
		})
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "anam.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

// A typo'd width must die at startup, where it costs a re-run. Absorbed on the ingest
// path instead, it would write months of real-looking bucket ids under a width nobody
// chose — and nothing downstream could tell them from correct ones.
func TestLoadRejectsAnUnknownBucketWidth(t *testing.T) {
	path := writeConfig(t, "person_id: matt\nbucket_width: fortnightly\n")

	_, err := config.Load(path)
	if !errors.Is(err, config.ErrInvalid) {
		t.Fatalf("Load() with bucket_width: fortnightly = %v, want it to wrap ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "bucket_width") {
		t.Errorf("error does not name the key: %v", err)
	}
}
