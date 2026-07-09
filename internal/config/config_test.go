package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mattjoyce/anam/internal/config"
)

func TestLoadDefaults(t *testing.T) {
	got, err := config.Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config.Config{
		PersonID:    "matt",
		TableName:   "anam",
		DDBEndpoint: "http://localhost:8000",
		DDBRegion:   "us-east-1",
		Timeout:     5 * time.Second,
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFromFile(t *testing.T) {
	path := writeConfig(t, `
person_id: matt
table_name: anam-test
ddb_endpoint: http://192.168.20.4:8000
ddb_region: ap-southeast-2
timeout: 12s
`)

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := config.Config{
		PersonID:    "matt",
		TableName:   "anam-test",
		DDBEndpoint: "http://192.168.20.4:8000",
		DDBRegion:   "ap-southeast-2",
		Timeout:     12 * time.Second,
	}
	if got != want {
		t.Errorf("Load() = %+v, want %+v", got, want)
	}
}

func TestLoadFileLeavesUnsetKeysAtDefaults(t *testing.T) {
	path := writeConfig(t, "table_name: anam-partial\n")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.TableName != "anam-partial" {
		t.Errorf("TableName = %q, want %q", got.TableName, "anam-partial")
	}
	if got.PersonID != "matt" {
		t.Errorf("PersonID = %q, want the default %q", got.PersonID, "matt")
	}
	if got.Timeout != 5*time.Second {
		t.Errorf("Timeout = %s, want the default 5s", got.Timeout)
	}
}

func TestEnvOverridesFileAndDefaults(t *testing.T) {
	path := writeConfig(t, "ddb_endpoint: http://from-file:8000\ntable_name: from-file\n")

	t.Setenv("ANAM_DDB_ENDPOINT", "http://from-env:8000")
	t.Setenv("ANAM_TIMEOUT", "30s")

	got, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.DDBEndpoint != "http://from-env:8000" {
		t.Errorf("DDBEndpoint = %q, want the environment to override the file", got.DDBEndpoint)
	}
	if got.Timeout != 30*time.Second {
		t.Errorf("Timeout = %s, want the environment to override the default", got.Timeout)
	}
	if got.TableName != "from-file" {
		t.Errorf("TableName = %q, want the file value where the environment is silent", got.TableName)
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
		TableName:   "anam",
		DDBEndpoint: "http://localhost:8000",
		DDBRegion:   "us-east-1",
		Timeout:     5 * time.Second,
	}

	tests := []struct {
		name    string
		mutate  func(*config.Config)
		wantErr bool
	}{
		{"valid", func(*config.Config) {}, false},
		{"missing person_id", func(c *config.Config) { c.PersonID = "" }, true},
		{"missing table_name", func(c *config.Config) { c.TableName = "" }, true},
		{"missing ddb_endpoint", func(c *config.Config) { c.DDBEndpoint = "" }, true},
		{"missing ddb_region", func(c *config.Config) { c.DDBRegion = "" }, true},
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
