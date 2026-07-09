package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
)

// TestLoadWithoutPersonIDFails guards the partition key. person_id has no default:
// defaulting it would let Anam write facets into a partition nobody chose.
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
		PersonID: "matt",
		DBPath:   "/data/anam/anam.db",
		Timeout:  5 * time.Second,
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestDefaultDBPath(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "/data")
	if got, want := config.DefaultDBPath(), "/data/anam/anam.db"; got != want {
		t.Errorf("DefaultDBPath() = %q, want %q (XDG_DATA_HOME honoured)", got, want)
	}

	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "/home/tester")
	if got, want := config.DefaultDBPath(), "/home/tester/.local/share/anam/anam.db"; got != want {
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
db_path: /srv/anam/ledger.db
timeout: 12s
`)

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config.Config{
		PersonID: "matt",
		DBPath:   "/srv/anam/ledger.db",
		Timeout:  12 * time.Second,
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFileLeavesUnsetKeysAtDefaults(t *testing.T) {
	path := writeConfig(t, "person_id: matt\ndb_path: /srv/partial.db\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.DBPath != "/srv/partial.db" {
		t.Errorf("DBPath = %q, want %q", got.DBPath, "/srv/partial.db")
	}
	if got.PersonID != "matt" {
		t.Errorf("PersonID = %q, want the file value %q", got.PersonID, "matt")
	}
	if got.Timeout != 5*time.Second {
		t.Errorf("Timeout = %s, want the default 5s", got.Timeout)
	}
}

func TestEnvOverridesFileAndDefaults(t *testing.T) {
	path := writeConfig(t, "person_id: from-file\ndb_path: /srv/from-file.db\n")

	t.Setenv("ANAM_DB_PATH", "/srv/from-env.db")
	t.Setenv("ANAM_TIMEOUT", "30s")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.DBPath != "/srv/from-env.db" {
		t.Errorf("DBPath = %q, want the environment to override the file", got.DBPath)
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
// believes it is writing to theirs.
func TestLoadRejectsUnknownKey(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"typo'd key", "db_pth: /srv/anam.db\n"},
		{"key from a future version", "seal_point: true\n"},
		{"camelCase instead of snake_case", "dbPath: /srv/anam.db\n"},
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
		PersonID: "matt",
		DBPath:   "/srv/anam.db",
		Timeout:  5 * time.Second,
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr bool
	}{
		{"valid", func(*config.Config) {}, false},
		{"missing person_id", func(c *config.Config) { c.PersonID = "" }, true},
		{"missing db_path", func(c *config.Config) { c.DBPath = "" }, true},
		{"zero timeout", func(c *config.Config) { c.Timeout = 0 }, true},
		{"negative timeout", func(c *config.Config) { c.Timeout = -time.Second }, true},
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
