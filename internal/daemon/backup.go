package daemon

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/backup"
	"github.com/mattjoyce/anam/internal/clock"
)

// postBackups takes a backup of the record into backup_dir, on the write queue: it
// waits behind a running mint or batch rather than refuse, since a scheduled backup
// has nowhere better to be, and holds the queue while it copies, so the ledger and the
// operational store in it agree. 503 without a backup_dir, or while stopping.
func (d *Daemon) postBackups(w http.ResponseWriter, r *http.Request) {
	if d.cfg.BackupDir == "" {
		writeError(w, http.StatusServiceUnavailable, errors.New("no backup_dir configured; set ANAM_BACKUP_DIR"))
		return
	}
	var b api.Backup
	err := d.Write(r.Context(), d.cfg.IngestTimeout, func(ctx context.Context, s Stores) error {
		var err error
		b, err = backup.Take(ctx, backup.Deps{
			Ledger: s.Ledger, Ops: s.Ops, ArtifactDir: d.cfg.ArtifactDir, LibraryDir: d.cfg.LibraryDir,
			Dir: d.cfg.BackupDir, Keep: d.cfg.BackupKeep, Clock: clock.Real{},
		})
		return err
	})
	switch {
	case errors.Is(err, ErrStopped):
		writeError(w, http.StatusServiceUnavailable, err)
	case err != nil:
		slog.Error("daemon: backup failed", "err", err)
		writeError(w, http.StatusInternalServerError, err)
	default:
		slog.Info("daemon: backup", "path", b.Path, "w", b.W, "files", b.Files, "bytes", b.Bytes, "pruned", b.Pruned)
		writeJSON(w, http.StatusOK, b)
	}
}
