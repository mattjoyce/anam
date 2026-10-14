package app

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/config"
)

func statusCmd(cfgPath *string, info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "the record's state: which ledger, its watermark, and what the daemon has seen",
		Long: `Status names the ledger and its watermark W. Asked of a daemon it also says which
build is running, when it started, and when each importer's last batch arrived
since then. Read from local files there is no daemon history to
report, so those parts are empty.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			var s api.Status
			if cfg.Server != "" {
				c, err := newClient(cfg)
				if err != nil {
					return err
				}
				s, err = c.Status(cmd.Context())
				if err != nil {
					return err
				}
			} else if s, err = localStatus(cmd.Context(), cfg, info); err != nil {
				return err
			}
			return writeStatus(cmd, s, cfg.Server)
		},
	}
}

// localStatus is the status of the local files: the ledger and W, and this build.
func localStatus(ctx context.Context, cfg config.Config, info BuildInfo) (api.Status, error) {
	l, err := openLedgerReadOnly(ctx, cfg)
	if err != nil {
		return api.Status{}, err
	}
	defer func() { _ = l.Close() }()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	id, err := l.ID(ctx)
	if err != nil {
		return api.Status{}, err
	}
	w, err := l.Head(ctx)
	if err != nil {
		return api.Status{}, err
	}
	return api.Status{Version: info.Version, Person: cfg.PersonID, LedgerID: id, W: int64(w)}, nil
}

func writeStatus(cmd *cobra.Command, s api.Status, server string) error {
	out := cmd.OutOrStdout()
	where := "local files"
	if server != "" {
		where = "daemon at " + server
	}
	_, _ = fmt.Fprintf(out, "ledger  %s (person %s)\n", s.LedgerID, s.Person)
	_, _ = fmt.Fprintf(out, "W       %d\n", s.W)
	_, _ = fmt.Fprintf(out, "build   %s, %s\n", s.Version, where)
	if s.Since == "" {
		return nil
	}
	_, _ = fmt.Fprintf(out, "since   %s\n", s.Since)
	if len(s.Sources) == 0 {
		_, err := fmt.Fprintln(out, "no batch from an importer since the daemon started")
		return err
	}
	_, _ = fmt.Fprintln(out)
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "SOURCE\tLAST BATCH\tLAST ADMISSION")
	for _, src := range s.Sources {
		last := src.LastAdmissionAt
		if last == "" {
			last = "-"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", src.Source, src.LastScanAt, last)
	}
	return w.Flush()
}
