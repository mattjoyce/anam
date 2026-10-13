// Package app is the anam CLI: command wiring only, no domain logic. main stays thin
// and calls Run (AGENTS.md, Structure).
package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/client"
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
	return newRoot(info).ExecuteContext(ctx)
}

// newRoot is the command tree.
func newRoot(info BuildInfo) *cobra.Command {
	var cfgPath string

	root := &cobra.Command{
		Use:           "anam",
		Short:         "anam — a personal, longitudinal context store",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&cfgPath, "config", "",
		"config file (default: $ANAM_CONFIG, then <user config dir>/anam/config.yaml)")
	root.PersistentFlags().String("server", "",
		"ask the daemon at this URL instead of opening local files (default: $ANAM_SERVER)")

	root.AddCommand(versionCmd(&cfgPath, info), statusCmd(&cfgPath, info), initCmd(&cfgPath), ingestCmd(&cfgPath),
		inspectCmd(&cfgPath), timelineCmd(&cfgPath), changesCmd(&cfgPath),
		serveCmd(&cfgPath), receiptCmd(&cfgPath), checkpointCmd(&cfgPath), retractCmd(&cfgPath), projectionCmd(&cfgPath),
		daemonCmd(&cfgPath, info))
	return root
}

// versionCmd reports this build and, when a server is configured, the daemon's. A
// config that does not load still gets the CLI's own line: the build is worth knowing
// most when something else is wrong.
func versionCmd(cfgPath *string, info BuildInfo) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "report the build, and the daemon's when a server is configured",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			_, err := fmt.Fprintf(out, "anam %s (commit %s, built %s)\n",
				info.Version, info.GitCommit, info.BuildDate)
			if err != nil {
				return err
			}
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "daemon not asked: %v\n", err)
				return nil
			}
			if cfg.Server == "" {
				return nil
			}
			c, err := newClient(cfg)
			if err != nil {
				return err
			}
			s, err := c.Status(cmd.Context())
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(out, "daemon %s at %s\n", s.Version, c.Server())
			return err
		},
	}
}

func inspectCmd(cfgPath *string) *cobra.Command {
	var listen string

	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "serve a read-only web view over the ledger, or print the daemon's",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			if cfg.Server != "" {
				// The daemon already serves the inspector, behind the same token as
				// the API. A second one here would read files the daemon owns.
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "%s/\n", strings.TrimSuffix(cfg.Server, "/"))
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

// loadConfig loads configuration for a command that opens local files. With a server
// configured those files are the daemon's, so it refuses rather than open them: a
// command not routed to the daemon never falls back to local files.
func loadConfig(explicit string, cmd *cobra.Command) (config.Config, error) {
	cfg, err := loadRoutedConfig(explicit, cmd)
	if err != nil {
		return config.Config{}, err
	}
	if cfg.Server != "" {
		return config.Config{}, fmt.Errorf("%s opens local files, and with server %s configured the daemon owns them; "+
			"to run it as maintenance, stop the daemon and unset server (ANAM_SERVER, --server)",
			cmd.CommandPath(), cfg.Server)
	}
	return cfg, nil
}

// loadRoutedConfig resolves and loads configuration, reporting where it came from —
// provenance for settings, same as for facets. It is for a command that asks the
// daemon when cfg.Server is set and opens local files only when it is not.
func loadRoutedConfig(explicit string, cmd *cobra.Command) (config.Config, error) {
	path, origin, err := config.Resolve(explicit)
	if err != nil {
		return config.Config{}, err
	}
	var set map[string]string
	if f := cmd.Flag("server"); f != nil && f.Changed {
		set = map[string]string{"server": f.Value.String()}
	}
	cfg, err := config.LoadOverriding(path, set)
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

// newClient is the client of cfg's server, presenting cfg's token.
func newClient(cfg config.Config) (*client.Client, error) {
	return client.New(cfg.Server, cfg.TokenFile, cfg.Timeout)
}
