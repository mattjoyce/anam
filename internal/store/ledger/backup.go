package ledger

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrBackupExists is returned when a backup's path already holds a file. A backup
// never writes over anything, least of all an earlier backup.
var ErrBackupExists = errors.New("ledger: a file is already at the backup path")

// BackupTo writes a consistent copy of the ledger to path, a file that must not exist,
// with SQLite's VACUUM INTO: one read transaction, so the copy is the ledger at one W
// even while it is held open. The copy carries the schema, pins and append-only
// triggers, and opens as this ledger.
func (l *Ledger) BackupTo(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%w: %s", ErrBackupExists, path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("ledger: backup to %s: %w", path, err)
	}
	if _, err := l.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("ledger: backup to %s: %w", path, err)
	}
	return nil
}
