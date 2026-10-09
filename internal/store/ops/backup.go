package ops

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ErrBackupExists is returned when a backup's path already holds a file.
var ErrBackupExists = errors.New("ops: a file is already at the backup path")

// BackupTo writes a consistent copy of the operational store to path, a file that
// must not exist, with SQLite's VACUUM INTO. Its authored tier (definitions, tokens)
// is only here, so a backup of the record carries it; the derived tier would replay.
func (d *DB) BackupTo(ctx context.Context, path string) error {
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%w: %s", ErrBackupExists, path)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("ops: backup to %s: %w", path, err)
	}
	if _, err := d.db.ExecContext(ctx, `VACUUM INTO ?`, path); err != nil {
		return fmt.Errorf("ops: backup to %s: %w", path, err)
	}
	return nil
}
