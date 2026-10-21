package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mattjoyce/anam/internal/config"
)

// isolate points the user config directory at a temp directory and clears
// $ANAM_CONFIG, so a test never reads the developer's real config. It returns
// the temp root and the file Resolve will look for: os.UserConfigDir decides
// where that is, and it is not $XDG_CONFIG_HOME everywhere — on macOS it is
// ~/Library/Application Support.
func isolate(t *testing.T) (root, candidate string) {
	t.Helper()

	root = t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	t.Setenv("HOME", root)
	t.Setenv(config.EnvPath, "")
	userDir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir: %v", err)
	}
	return root, filepath.Join(userDir, "anam", "config.yaml")
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

// real is the path safepath resolves to: symlinks followed, so on a system where
// the temp directory is reached through a symlink (/var on macOS) the file that
// is opened is named by its real path.
func real(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve %q: %v", path, err)
	}
	return resolved
}

func TestResolveExplicitPath(t *testing.T) {
	isolate(t)
	want := touch(t, filepath.Join(t.TempDir(), "custom.yaml"))

	got, origin, err := config.Resolve(want)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != real(t, want) {
		t.Errorf("Resolve() path = %q, want %q", got, real(t, want))
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
	if got != real(t, want) {
		t.Errorf("Resolve() path = %q, want %q", got, real(t, want))
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
	_, candidate := isolate(t)
	want := touch(t, candidate)

	got, origin, err := config.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != real(t, want) {
		t.Errorf("Resolve() path = %q, want %q", got, real(t, want))
	}
	if origin != config.OriginUserDir {
		t.Errorf("Resolve() origin = %q, want %q", origin, config.OriginUserDir)
	}
}

// TestResolveHonoursUserConfigDir pins the reason for using os.UserConfigDir rather
// than a hardcoded ~/.config: a moved config root must be respected, wherever
// os.UserConfigDir says the root is.
func TestResolveHonoursUserConfigDir(t *testing.T) {
	root, candidate := isolate(t)
	want := touch(t, candidate)

	got, _, err := config.Resolve("")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if got != real(t, want) {
		t.Errorf("Resolve() path = %q, want the user-config-dir path %q", got, real(t, want))
	}
	if !strings.HasPrefix(got, real(t, root)+string(filepath.Separator)) {
		t.Errorf("Resolve() ignored the user config dir: %q is not under %q", got, root)
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
	_, candidate := isolate(t)
	inUserDir := touch(t, candidate)
	fromEnv := touch(t, filepath.Join(t.TempDir(), "env.yaml"))
	explicit := touch(t, filepath.Join(t.TempDir(), "flag.yaml"))

	t.Setenv(config.EnvPath, fromEnv)

	// Explicit beats environment beats user config dir.
	if got, _, _ := config.Resolve(explicit); got != real(t, explicit) {
		t.Errorf("Resolve(explicit) = %q, want the explicit path %q", got, explicit)
	}
	if got, _, _ := config.Resolve(""); got != real(t, fromEnv) {
		t.Errorf("Resolve(\"\") = %q, want the $ANAM_CONFIG path %q", got, fromEnv)
	}

	t.Setenv(config.EnvPath, "")
	if got, _, _ := config.Resolve(""); got != real(t, inUserDir) {
		t.Errorf("Resolve(\"\") = %q, want the user-config-dir path %q", got, inUserDir)
	}
}

// TestResolveThenLoad exercises the two halves together, the way a command will.
func TestResolveThenLoad(t *testing.T) {
	_, candidate := isolate(t)
	t.Setenv("ANAM_PERSON_ID", "")

	path := candidate
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
