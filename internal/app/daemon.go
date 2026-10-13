package app

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/daemon"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/gate"
)

func daemonCmd(cfgPath *string, info BuildInfo) *cobra.Command {
	var importsPath string

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "run Anam as one long-running process that owns the ledger",
		Long: `Daemon opens the ledger and the operational store once and keeps them, so that
while it runs it is the only process writing them (docs/API.md §2). Writes run one at
a time; reads use a read-only handle.

It serves on the configured address (listen, ANAM_LISTEN):

    GET  /healthz                       ready or not, without a token
    POST /v1/ingest                     run the declared imports; 202 with an operation
    GET  /v1/operations/{id}            that run's state, tallies and W before and after
    GET  /v1/status                     ledger id, W, and each source as last seen
    GET  /v1/timeline                   the timeline read, with the reader's zone
    GET  /v1/changes                    anam-events in a window of anam-time
    GET  /v1/receipts[/{hash}[/verify]] receipts: list, show, prove
    POST /v1/receipts                   publish a timeline read through a projection, with a receipt
    GET  /v1/projections[/{name}[/{v}]] the projection templates: list, newest, one version
    POST /v1/projections                register a template the operator wrote
    GET  /v1/checkpoints                the checkpoint log
    POST /v1/checkpoints                mint (?dry_run=true to derive only)
    POST /v1/backups                    back up the record into backup_dir
    GET  /                              the inspector

and to importers, each under its own token (importers_file, docs/IMPORTERS.md §8):

    POST /v1/envelopes                  a batch of envelopes, admitted whole or refused whole
    POST /v1/definitions                register a Definition the importer emits
    GET  /v1/definitions[/{type}[/{v}]] the published Definitions (the operator may read too)

The CLI asks these when server is set (ANAM_SERVER, --server); see anam --help.

With token_file set, everything but /healthz and the importer routes needs the token, as a bearer token or
as a basic-auth password, and the daemon may listen on any address. Without it the
daemon listens on loopback only.

The imports file is read at every run, so a change to it needs no restart.

SIGTERM or Ctrl-C stops it: requests in flight finish, the running write finishes
or reaches its deadline, and the stores are closed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			d, err := daemon.Open(cmd.Context(), cfg, daemon.Options{
				Ingest:  daemonIngest(cfg, importsPath),
				Version: info.Version,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "daemon: ledger %s (person %s)\nhttp://%s/\n",
				cfg.LedgerPath, cfg.PersonID, cfg.Listen)
			return d.Run(cmd.Context())
		},
	}
	cmd.Flags().StringVar(&importsPath, "imports", "",
		"imports file (default: $ANAM_IMPORTS, then <user config dir>/anam/imports.yaml)")
	cmd.AddCommand(daemonReadyCmd(cfgPath))
	return cmd
}

// daemonReadyCmd asks a running daemon's /healthz and exits by its answer. It is the
// container's health check: a distroless image has no curl to do it with.
func daemonReadyCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "ready",
		Short: "exit 0 if the daemon on the configured address answers ready",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()
			return daemon.Ready(ctx, cfg.Listen)
		},
	}
}

// daemonIngest is the CLI's ingest on the daemon's held stores: the same guard, the
// same read-everything-first scan, the same gate and scopes. It differs only in where
// the handles come from and where the tallies go.
func daemonIngest(cfg config.Config, importsPath string) daemon.IngestFunc {
	return func(ctx context.Context, s daemon.Stores) ([]api.SourceResult, error) {
		path, origin, err := config.ResolveImports(importsPath)
		if err != nil {
			return nil, err
		}
		if path == "" {
			return nil, fmt.Errorf("no imports file found: declare imports in %s or pass --imports",
				"<user config dir>/anam/imports.yaml")
		}
		slog.Info("daemon: ingest", "imports", path, "origin", origin)
		imports, err := config.LoadImports(path)
		if err != nil {
			return nil, err
		}
		imports = withLibrary(imports, cfg)

		reg, err := def.Bundled()
		if err != nil {
			return nil, err
		}
		guardCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		err = guardWith(guardCtx, s.Ops, reg)
		cancel()
		if err != nil {
			return nil, err
		}
		all, err := scanAll(ctx, imports, cfg)
		if err != nil {
			return nil, err
		}

		g := gate.New(s.Ledger, reg, mvpScopes, clock.Real{}, cfg.PersonID)
		var out []api.SourceResult
		_, err = admitSources(ctx, g, all, cfg.Timeout, gate.MaxBatch, func(sc scanned, p progress) {
			out = append(out, resultOf(sc.source, len(sc.envs), p))
		})
		return out, err
	}
}
