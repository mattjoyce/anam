// Package backup takes a backup of the record: the ledger, the operational store and the
// library, into one directory that restores by copying it back.
//
// The record is the ledger directory (ARCHITECTURE.md §8), and the daemon holds its
// databases open, so a file copy could catch one mid-write. Each store copies itself
// with SQLite's VACUUM INTO, one read transaction each, and the caller takes the
// backup inside one write, so nothing is admitted between the two copies and they
// agree. The library is plain files, copied as they are. The operational store carries
// the receipts, served values and saved selections, so it is the whole of what Anam
// authored beside the ledger.
//
// Failure and recovery (Armstrong): a backup is written under a .partial name and
// renamed when complete, so a crash leaves a .partial directory, never a backup that
// looks whole and is not. The next backup removes it. Pruning removes only names this
// package wrote, so nothing else in the directory is touched.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// Ledger is what a backup needs of the ledger.
type Ledger interface {
	BackupTo(ctx context.Context, path string) error
	Head(ctx context.Context) (ledger.Watermark, error)
}

// Store is what a backup needs of the operational store.
type Store interface {
	BackupTo(ctx context.Context, path string) error
}

// Clock names the backup; tests freeze it.
type Clock interface{ Now() time.Time }

// Deps is everything a backup needs.
type Deps struct {
	Ledger     Ledger
	Ops        Store
	LibraryDir string
	// Dir is where backups are kept, one directory each; Keep is how many.
	Dir   string
	Keep  int
	Clock Clock
}

// layout names a backup by the UTC second it was taken.
const layout = "20060102T150405Z"

var (
	named   = regexp.MustCompile(`^anam-\d{8}T\d{6}Z$`)
	partial = regexp.MustCompile(`^anam-\d{8}T\d{6}Z\.partial$`)
)

// Take writes a new backup into d.Dir and prunes the oldest beyond d.Keep.
func Take(ctx context.Context, d Deps) (api.Backup, error) {
	if d.Dir == "" {
		return api.Backup{}, errors.New("backup: no backup_dir configured")
	}
	if d.Keep < 1 {
		return api.Backup{}, fmt.Errorf("backup: keep must be at least 1, got %d", d.Keep)
	}
	if err := os.MkdirAll(d.Dir, 0o750); err != nil {
		return api.Backup{}, fmt.Errorf("backup: %w", err)
	}
	if err := sweep(d.Dir); err != nil {
		return api.Backup{}, err
	}

	name := "anam-" + d.Clock.Now().UTC().Format(layout)
	final := filepath.Join(d.Dir, name)
	tmp := final + ".partial"
	if err := os.Mkdir(tmp, 0o750); err != nil {
		return api.Backup{}, fmt.Errorf("backup: %w", err)
	}
	w, err := d.write(ctx, tmp)
	if err != nil {
		_ = os.RemoveAll(tmp) // the error being returned is the one that matters
		return api.Backup{}, err
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.RemoveAll(tmp) // as above
		return api.Backup{}, fmt.Errorf("backup: %w", err)
	}

	out := api.Backup{Path: final, W: int64(w)}
	if out.Files, out.Bytes, err = measure(final); err != nil {
		return api.Backup{}, err
	}
	if out.Pruned, err = prune(d.Dir, d.Keep); err != nil {
		return out, err
	}
	return out, nil
}

// write fills dir: the two stores, then the library. It returns the
// W the ledger copy holds.
func (d Deps) write(ctx context.Context, dir string) (ledger.Watermark, error) {
	w, err := d.Ledger.Head(ctx)
	if err != nil {
		return 0, err
	}
	if err := d.Ledger.BackupTo(ctx, filepath.Join(dir, "ledger.db")); err != nil {
		return 0, err
	}
	if err := d.Ops.BackupTo(ctx, filepath.Join(dir, "tracking.db")); err != nil {
		return 0, err
	}
	if err := copyTree(d.LibraryDir, filepath.Join(dir, "documents")); err != nil {
		return 0, err
	}
	return w, nil
}

// copyTree copies the tree at src to dst. A src that does not exist is an empty tree:
// no library yet.
func copyTree(src, dst string) error {
	if src == "" {
		return nil
	}
	if _, err := os.Stat(src); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err := os.CopyFS(dst, os.DirFS(src)); err != nil {
		return fmt.Errorf("backup: copy %s: %w", src, err)
	}
	return nil
}

// measure counts the files and bytes under dir.
func measure(dir string) (files int, bytes int64, err error) {
	err = filepath.WalkDir(dir, func(_ string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		info, err := e.Info()
		if err != nil {
			return err
		}
		files++
		bytes += info.Size()
		return nil
	})
	if err != nil {
		return 0, 0, fmt.Errorf("backup: measure %s: %w", dir, err)
	}
	return files, bytes, nil
}

// sweep removes the .partial directories a crashed backup left.
func sweep(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() && partial.MatchString(e.Name()) {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return fmt.Errorf("backup: remove %s: %w", e.Name(), err)
			}
		}
	}
	return nil
}

// prune removes the oldest backups beyond keep and returns their names. Names sort by
// the time they were taken.
func prune(dir string, keep int) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("backup: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && named.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) <= keep {
		return nil, nil
	}
	old := names[:len(names)-keep]
	for _, n := range old {
		if err := os.RemoveAll(filepath.Join(dir, n)); err != nil {
			return nil, fmt.Errorf("backup: remove %s: %w", n, err)
		}
	}
	return old, nil
}

// String renders a backup for a person: where, at what W, how big.
func String(b api.Backup) string {
	s := fmt.Sprintf("backup %s at W=%d: %d files, %.1f MB", b.Path, b.W, b.Files, float64(b.Bytes)/1e6)
	if len(b.Pruned) > 0 {
		s += "; removed " + strings.Join(b.Pruned, ", ")
	}
	return s
}
