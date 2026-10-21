// Package safepath is Anam's one place for handling operator-supplied file paths.
// Every path that arrives from outside — a flag, an environment variable, a config
// or import declaration — goes through here before anything opens it: expanded
// (~ and $VARs), made absolute, cleaned, and — for files that will be read —
// symlink-resolved and required to be a regular file.
//
// Concentrating this in one package means exactly one audited os.Open of a
// variable path exists in the codebase, instead of a #nosec scattered per worker.
package safepath

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotRegular is returned when a resolved path names something other than a
// regular file — a directory, device, socket or pipe. Readers read files.
var ErrNotRegular = errors.New("safepath: not a regular file")

// Expand normalises an operator-supplied path without requiring it to exist:
// $VARs and a leading ~ are expanded, and the result is absolute and cleaned.
// Use it for paths that will be created (a database file, an artifact directory).
func Expand(path string) (string, error) {
	if path == "" {
		return "", errors.New("safepath: empty path")
	}
	path = os.ExpandEnv(path)

	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("safepath: expand %q: %w", path, err)
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	// ~user is a shell convention this deliberately does not implement; refusing
	// beats silently treating it as a relative directory named "~user".
	if strings.HasPrefix(path, "~") {
		return "", fmt.Errorf("safepath: %q: ~user expansion is not supported", path)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("safepath: absolutise %q: %w", path, err)
	}
	return abs, nil
}

// ResolveFile pins a path that is about to be read: Expand, then symlinks
// resolved to the real file, which must exist and be a regular file. The returned
// path is the one that will actually be opened — no symlink swap between check
// and use can point it anywhere the resolution didn't already see.
//
// The check is confined as well as resolved: the file is stat'ed through a root
// opened on its own directory, with the name alone. Nothing in the path can walk
// out of that directory between the resolution and the check, because the check
// never names a path. It names a file beside the directory it was resolved in.
func ResolveFile(path string) (string, error) {
	abs, err := Expand(path)
	if err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("safepath: resolve %q: %w", abs, err)
	}
	dir, name := filepath.Dir(real), filepath.Base(real)
	root, err := os.OpenRoot(dir)
	if err != nil {
		return "", fmt.Errorf("safepath: open root %q: %w", dir, err)
	}
	defer func() { _ = root.Close() }()
	info, err := root.Stat(name)
	if err != nil {
		return "", fmt.Errorf("safepath: stat %q: %w", real, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %q is %s", ErrNotRegular, real, info.Mode().Type())
	}
	return real, nil
}

// Open resolves and opens a file for reading — the codebase's one os.Open of a
// variable path.
func Open(path string) (*os.File, error) {
	real, err := ResolveFile(path)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(real) // #nosec G304 -- real is ResolveFile-pinned: expanded, symlink-resolved, a regular file
	if err != nil {
		return nil, fmt.Errorf("safepath: open %q: %w", real, err)
	}
	return f, nil
}
