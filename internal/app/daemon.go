package app

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/daemon"
)

func daemonCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "run Anam as one long-running process that owns the ledger",
		Long: `Daemon opens the ledger and the operational store once and keeps them, so that
while it runs it is the only process writing them (docs/API.md §2). Writes run one at
a time; reads use a read-only handle. It serves the inspector and /healthz on the
configured address (listen, ANAM_LISTEN), which must be loopback for now.

SIGTERM or Ctrl-C stops it: requests in flight finish, the running write finishes
or reaches its deadline, and the stores are closed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			d, err := daemon.Open(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "daemon: ledger %s (person %s)\nhttp://%s/\n",
				cfg.LedgerPath, cfg.PersonID, cfg.Listen)
			return d.Run(cmd.Context())
		},
	}
}
