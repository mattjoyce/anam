package app

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/daemon"
)

func daemonCmd(cfgPath *string, info BuildInfo) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "run Anam as one long-running process that owns the ledger",
		Long: `Daemon opens the ledger and the operational store once and keeps them, so that
while it runs it is the only process writing them (docs/API.md §2). Writes run one at
a time; reads use a read-only handle.

It serves on the configured address (listen, ANAM_LISTEN):

    GET  /healthz                       ready or not, without a token
    GET  /v1/openapi.yaml               the routes, with roles, parameters and bodies, without a token
    GET  /v1/status                     ledger id, W, and each importer's last batch
    GET  /v1/timeline                   the timeline read, with the reader's zone
    GET  /v1/changes                    anam-events in a window of anam-time
    GET  /v1/coverage                   each type's count, fact-time range and sources at a W
    GET  /v1/receipts[/{hash}[/verify]] receipts: list, show, prove
    GET  /v1/receipts/{hash}/facets/{facet_id}  one facet as the receipt served it
    POST /v1/receipts                   publish a timeline read through a projection, with a receipt
    GET  /v1/snapshots/{id}             an importer's snapshot run: its parts and whether it is complete
    GET  /v1/query                      the query catalogue
    POST /v1/query                      run a query over what a projection serves
    POST /v1/selections                 save a selection and get its short code
    GET  /v1/selections/{code}          read a saved selection
    GET  /v1/projections[/{name}[/{v}]] the projection templates: list, newest, one version
    POST /v1/projections                register a template the operator wrote
    GET  /v1/checkpoints                the checkpoint log
    POST /v1/checkpoints                mint (?dry_run=true to derive only)
    POST /v1/backups                    back up the record into backup_dir
    POST /v1/hold, DELETE /v1/hold      hold a source's admissions, or release it
    POST /v1/facets/{id}/retract        tombstone one facet by hand
    POST /v1/entry                      enter one fact by hand, as the workbench source
    GET  /                              the inspector

and to importers, each under its own token (importers_file, docs/IMPORTERS.md §8):

    POST /v1/envelopes                  a batch of envelopes, admitted whole or refused whole
    POST /v1/definitions                register a Definition the importer emits
    POST /v1/documents?ext=             store a document's bytes in the library
    GET  /v1/definitions[/{type}[/{v}]] the published Definitions (the operator may read too)

The CLI asks these when server is set (ANAM_SERVER, --server); see anam --help.

A browser on another site cannot make the daemon write: a POST that carries a foreign
Origin or a Sec-Fetch-Site other than same-origin is refused with 403 before any work,
whatever credential it carries, and a tokenless daemon also refuses a POST whose Host is
not a loopback name. The CLI, importers and the inspector's own forms are unaffected.

With token_file set, every route but /healthz and /v1/openapi.yaml needs a token, as a bearer token or
as a basic-auth password, and the daemon may listen on any address. A reader's token opens the
reads, an importer's the reads and its own submissions, and the operator's the reads, the operator's
writes above and the inspector (docs/API.md §2). Without token_file the daemon listens on loopback only.

Every source reaches the record through an importer; the daemon reads no source
itself.

SIGTERM or Ctrl-C stops it: requests in flight finish, the running write finishes
or reaches its deadline, and the stores are closed.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			d, err := daemon.Open(cmd.Context(), cfg, daemon.Options{Version: info.Version, GitCommit: info.GitCommit, BuildDate: info.BuildDate})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "daemon: ledger %s (person %s)\nhttp://%s/\n",
				cfg.LedgerPath, cfg.PersonID, cfg.Listen)
			return d.Run(cmd.Context())
		},
	}
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
