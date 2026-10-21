package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/mattjoyce/anam/internal/safepath"
)

// EnvPath names the environment variable that overrides config discovery.
const EnvPath = "ANAM_CONFIG"

// ErrNotFound is returned when a config file was named explicitly but is absent.
var ErrNotFound = errors.New("config file not found")

// Origin records where a config file path came from. Anam is a provenance system;
// "where did this setting come from" deserves the same answer as "where did this facet
// come from".
type Origin string

// The origins, in search order. OriginDefaults means no file was found and none was
// asked for.
const (
	OriginFlag     Origin = "flag"
	OriginEnv      Origin = "env:" + EnvPath
	OriginUserDir  Origin = "user-config-dir"
	OriginDefaults Origin = "defaults"
)

// Resolve finds the config file to load, searching in order: the explicit path, then
// $ANAM_CONFIG, then <user-config-dir>/anam/config.yaml.
//
// The two cases are deliberately asymmetric. A path named **explicitly** — by flag or
// by environment — that does not exist is an error: it was asked for, it is absent,
// say so. A **discovered** path that does not exist is not an error: nothing was
// promised, and Anam runs on defaults.
//
// The user config directory comes from os.UserConfigDir, which honours
// $XDG_CONFIG_HOME where that is the convention and falls back to the platform's
// own place otherwise (~/.config on Linux, ~/Library/Application Support on
// macOS). Hardcoding one of them would silently ignore anyone who has moved
// their config root.
//
// A returned path of "" means: load defaults, read no file. A returned path is the
// file that will be read — expanded, absolute, symlinks resolved — so what a
// caller reports is what it opened.
func Resolve(explicit string) (string, Origin, error) {
	if explicit != "" {
		path, err := mustExist(explicit)
		if err != nil {
			return "", "", err
		}
		return path, OriginFlag, nil
	}

	if fromEnv := os.Getenv(EnvPath); fromEnv != "" {
		path, err := mustExist(fromEnv)
		if err != nil {
			return "", "", fmt.Errorf("%s: %w", EnvPath, err)
		}
		return path, OriginEnv, nil
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		// No HOME and no XDG_CONFIG_HOME. Not fatal: nothing was asked for.
		return "", OriginDefaults, nil
	}

	candidate := filepath.Join(dir, "anam", "config.yaml")
	path, err := mustExist(candidate)
	switch {
	case err == nil:
		return path, OriginUserDir, nil
	case errors.Is(err, ErrNotFound):
		return "", OriginDefaults, nil
	default:
		return "", "", err
	}
}

// mustExist resolves an operator-supplied path and requires it to be a file that
// is there, returning the path that will be opened — the real one, so a caller
// reports the same string it read and no symlink can be swapped between the
// check and the read.
//
// The path handling is safepath's, as it is for every other operator-supplied
// path: expanded, absolute, symlinks resolved, and the file checked inside its
// own directory.
func mustExist(path string) (string, error) {
	real, err := safepath.ResolveFile(path)
	switch {
	case err == nil:
		return real, nil
	case errors.Is(err, os.ErrNotExist):
		return "", fmt.Errorf("%w: %q", ErrNotFound, path)
	default:
		return "", fmt.Errorf("config file %q: %w", path, err)
	}
}
