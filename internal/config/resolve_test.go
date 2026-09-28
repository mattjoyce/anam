package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
)

// isolate points os.UserConfigDir at a temp directory and clears $ANAM_CONFIG, so a
// test never reads the developer's real config.
func isolate(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv(config.EnvPath, "")
	return dir
}

// touch creates an empty file, making the parent directories as needed.
func touch(t *testing.T, path string) string {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func TestResolveExplicitPath(t *testing.T) {
	isolate(t)
	want := touch(t, filepath.Join(t.TempDir(), "custom.yaml"))

	got, origin, err := config.Resolve(want)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != want {
		t.Errorf("Resolve() path = %q, want %q", got, want)
	}
	if origin != config.OriginFlag {
		t.Errorf("Resolve() origin = %q, want %q", origin, config.OriginFlag)
	}
}

// TestResolveExplicitPathMissing is the asymmetry: a path asked for by name and absent
// is an error, never a silent fallback to defaults.
func TestResolveExplicitPathMissing(t *testing.T) {
	isolate(t)
	absent := filepath.Join(t.TempDir(), "absent.yaml")

	_, _, err := config.Resolve(absent)
	if err == nil {
		t.Fatal("Resolve() = nil error for an explicitly named missing file, want error")
	}
	if !errors.Is(err, config.ErrNotFound) {
		t.Errorf("Resolve() error = %v, want it to wrap ErrNotFound", err)
	}
}

func TestResolveFromEnv(t *testing.T) {
	isolate(t)
	want := touch(t, filepath.Join(t.TempDir(), "from-env.yaml"))
	t.Setenv(config.EnvPath, want)

	got, origin, err := config.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != want {
		t.Errorf("Resolve() path = %q, want %q", got, want)
	}
	if origin != config.OriginEnv {
		t.Errorf("Resolve() origin = %q, want %q", origin, config.OriginEnv)
	}
}

func TestResolveFromEnvMissing(t *testing.T) {
	isolate(t)
	t.Setenv(config.EnvPath, filepath.Join(t.TempDir(), "absent.yaml"))

	_, _, err := config.Resolve("")
	if err == nil {
		t.Fatal("Resolve() = nil error for a missing $ANAM_CONFIG, want error")
	}
	if !errors.Is(err, config.ErrNotFound) {
		t.Errorf("Resolve() error = %v, want it to wrap ErrNotFound", err)
	}
}

func TestResolveFromUserConfigDir(t *testing.T) {
	dir := isolate(t)
	want := touch(t, filepath.Join(dir, "anam", "config.yaml"))

	got, origin, err := config.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != want {
		t.Errorf("Resolve() path = %q, want %q", got, want)
	}
	if origin != config.OriginUserDir {
		t.Errorf("Resolve() origin = %q, want %q", origin, config.OriginUserDir)
	}
}

// TestResolveHonoursXDGConfigHome pins the reason for using os.UserConfigDir rather
// than a hardcoded ~/.config: a moved config root must be respected.
func TestResolveHonoursXDGConfigHome(t *testing.T) {
	dir := isolate(t)
	want := touch(t, filepath.Join(dir, "anam", "config.yaml"))

	got, _, err := config.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != want {
		t.Errorf("Resolve() path = %q, want the XDG_CONFIG_HOME path %q", got, want)
	}
	if filepath.Dir(filepath.Dir(got)) != dir {
		t.Errorf("Resolve() ignored XDG_CONFIG_HOME: %q is not under %q", got, dir)
	}
}

// TestResolveNoFileAnywhere is the other half of the asymmetry: nothing was asked for,
// so nothing found is not an error. Anam runs on defaults.
func TestResolveNoFileAnywhere(t *testing.T) {
	isolate(t)

	got, origin, err := config.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil when no file was asked for", err)
	}
	if got != "" {
		t.Errorf("Resolve() path = %q, want \"\" meaning load defaults", got)
	}
	if origin != config.OriginDefaults {
		t.Errorf("Resolve() origin = %q, want %q", origin, config.OriginDefaults)
	}
}

func TestResolvePrecedence(t *testing.T) {
	dir := isolate(t)
	inUserDir := touch(t, filepath.Join(dir, "anam", "config.yaml"))
	fromEnv := touch(t, filepath.Join(t.TempDir(), "env.yaml"))
	explicit := touch(t, filepath.Join(t.TempDir(), "flag.yaml"))

	t.Setenv(config.EnvPath, fromEnv)

	// Explicit beats environment beats user config dir.
	if got, _, _ := config.Resolve(explicit); got != explicit {
		t.Errorf("Resolve(explicit) = %q, want the explicit path %q", got, explicit)
	}
	if got, _, _ := config.Resolve(""); got != fromEnv {
		t.Errorf("Resolve(\"\") = %q, want the $ANAM_CONFIG path %q", got, fromEnv)
	}

	t.Setenv(config.EnvPath, "")
	if got, _, _ := config.Resolve(""); got != inUserDir {
		t.Errorf("Resolve(\"\") = %q, want the user-config-dir path %q", got, inUserDir)
	}
}

// TestResolveThenLoad exercises the two halves together, the way a command will.
func TestResolveThenLoad(t *testing.T) {
	dir := isolate(t)
	t.Setenv("ANAM_PERSON_ID", "")

	path := filepath.Join(dir, "anam", "config.yaml")
	touch(t, path)
	if err := os.WriteFile(path, []byte("person_id: matt\nledger_path: /srv/xdg/ledger.db\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	resolved, origin, err := config.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	cfg, err := config.Load(resolved)
	if err != nil {
		t.Fatalf("Load(%q) error = %v", resolved, err)
	}

	if origin != config.OriginUserDir {
		t.Errorf("origin = %q, want %q", origin, config.OriginUserDir)
	}
	if cfg.PersonID != "matt" || cfg.LedgerPath != "/srv/xdg/ledger.db" {
		t.Errorf("Load() = %+v, want person_id=matt ledger_path=/srv/xdg/ledger.db", cfg)
	}
}
