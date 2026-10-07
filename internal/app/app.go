// Package app is the anam CLI: command wiring only, no domain logic. main stays thin
// and calls Run (AGENTS.md, Structure).
package app

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/inspect"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// BuildInfo carries the ldflags-injected identity of this binary.
type BuildInfo struct {
	Version   string
	GitCommit string
	BuildDate string
}

// Run executes the CLI.
func Run(ctx context.Context, info BuildInfo) error {
	var cfgPath string

	root := &cobra.Command{
		Use:           "anam",
		Short:         "anam — a personal, longitudinal context store",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "",
		"config file (default: $ANAM_CONFIG, then <user config dir>/anam/config.yaml)")

	root.AddCommand(versionCmd(info), initCmd(&cfgPath), ingestCmd(&cfgPath),
		inspectCmd(&cfgPath), timelineCmd(&cfgPath), changesCmd(&cfgPath),
		serveCmd(&cfgPath), receiptCmd(&cfgPath), checkpointCmd(&cfgPath), retractCmd(&cfgPath))
	return root.ExecuteContext(ctx)
}

func versionCmd(info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "report the build",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "anam %s (commit %s, built %s)\n",
				info.Version, info.GitCommit, info.BuildDate)
			return err
		},
	}
}

func inspectCmd(cfgPath *string) *cobra.Command {
	var listen string

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "serve a read-only web view over the ledger (diagnostic harness)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}

			l, err := openLedgerReadOnly(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer func() { _ = l.Close() }()
			buckets, err := cfg.Buckets()
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "inspecting ledger %s (person %s), read-only\nhttp://%s/\n",
				cfg.LedgerPath, cfg.PersonID, listen)
			return inspect.Serve(cmd.Context(), l, listen, cfg.Timeout, buckets, cfg.LibraryDir)
		},
	}
	// Loopback by default: the inspector is a local diagnostic tool, not a service.
	cmd.Flags().StringVar(&listen, "listen", "127.0.0.1:8787", "address to serve on")
	return cmd
}

// openLedgerReadOnly opens the ledger the way every reading command does: on a handle
// SQLite will not accept a write from. Only init and ingest open it for writing.
func openLedgerReadOnly(ctx context.Context, cfg config.Config) (*ledger.Ledger, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return ledger.OpenReadOnly(ctx, cfg.LedgerPath, cfg.PersonID)
}

// loadConfig resolves and loads configuration, reporting where it came from —
// provenance for settings, same as for facets.
func loadConfig(explicit string, cmd *cobra.Command) (config.Config, error) {
	path, origin, err := config.Resolve(explicit)
	if err != nil {
		return config.Config{}, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, err
	}
	if path != "" {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "config: %s (%s)\n", path, origin)
	} else {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "config: defaults (no file found)")
	}
	return cfg, nil
}
