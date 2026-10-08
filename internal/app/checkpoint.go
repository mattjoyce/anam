package app

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func checkpointCmd(cfgPath *string) *cobra.Command {
	var q api.MintQuery

	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "mint a checkpoint per bucket whose value has changed",
		Long: `Checkpoint folds the ledger at its head watermark, splits the live value by bucket,
and writes a new version of every bucket whose value differs from its last
checkpoint. A bucket that has not changed is left alone: same hash, no new row.

Each value is written to the artifact directory under the SHA-256 of its own bytes,
and the checkpoint log in the operational store records the bucket, its version and
the watermark it folded. Minting writes nothing to the ledger; a checkpoint is a
cache of the fold, there to bound replay, and is rebuilt by running this again.

Buckets subdivide: a bucket whose value is over checkpoint_budget bytes (default
1 MiB) is split along the calendar, a year into months, a month into days. The run
is recorded, and a timeline read then starts from these checkpoints and replays only
what came after, so mint again after an ingest to keep reads short.

A mint starts from the last run: only the buckets the events since then touched are
rebuilt and compared. --full folds the whole ledger and compares every bucket instead,
which repairs any checkpoint that is not the fold's.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			m, err := runMint(cmd.Context(), cfg, q)
			if err != nil {
				return err
			}
			return writeMint(cmd, m)
		},
	}
	cmd.Flags().BoolVar(&q.DryRun, "dry-run", false, "derive and report, but publish nothing")
	cmd.Flags().BoolVar(&q.Full, "full", false, "fold the whole ledger rather than start from the last run")
	cmd.AddCommand(checkpointListCmd(cfgPath))
	return cmd
}

func checkpointListCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list every checkpoint ever minted, by bucket and version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			list, err := listCheckpoints(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			return writeCheckpoints(cmd, list)
		},
	}
}

// runMint asks the daemon to mint when a server is configured, and mints into the
// local files when not.
func runMint(ctx context.Context, cfg config.Config, q api.MintQuery) (api.Mint, error) {
	if cfg.Server != "" {
		c, err := newClient(cfg)
		if err != nil {
			return api.Mint{}, err
		}
		return c.Mint(ctx, q)
	}
	deps, closeAll, err := mintDeps(ctx, cfg)
	if err != nil {
		return api.Mint{}, err
	}
	defer closeAll()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return mint.Report(ctx, deps, q)
}

func writeMint(cmd *cobra.Command, m api.Mint) error {
	out := cmd.OutOrStdout()
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "BUCKET\tV\tCOVERS W\tFACETS\tVALUE\t")
	for _, o := range m.Outcomes {
		note := ""
		switch {
		case o.Minted && !m.Written:
			note = "would mint"
		case o.Minted:
			note = "minted"
		default:
			note = "unchanged"
		}
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%d\t%s\t%s\n",
			o.SpanID, o.V, o.CoversW, o.Len, o.ValueHash, note)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	verb := "minted"
	if !m.Written {
		verb = "would mint (dry run — nothing written)"
	}
	from := "folding the whole ledger"
	if !m.Full {
		from = fmt.Sprintf("from the run at W=%d", m.From)
	}
	_, err := fmt.Fprintf(out, "%s %d of %d buckets at W=%d, %s\n",
		verb, m.MintedCount(), len(m.Outcomes), m.W, from)
	return err
}

// listCheckpoints asks the daemon when a server is configured. Locally it reads the
// checkpoint log and nothing else, so it still answers when there is no ledger.
func listCheckpoints(ctx context.Context, cfg config.Config) (api.Checkpoints, error) {
	if cfg.Server != "" {
		c, err := newClient(cfg)
		if err != nil {
			return api.Checkpoints{}, err
		}
		return c.Checkpoints(ctx)
	}
	buckets, err := cfg.Buckets()
	if err != nil {
		return api.Checkpoints{}, err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		return api.Checkpoints{}, err
	}
	defer func() { _ = db.Close() }()
	return read.Checkpoints(ctx, db, cfg.DBPath)
}

func writeCheckpoints(cmd *cobra.Command, list api.Checkpoints) error {
	if len(list.Checkpoints) == 0 {
		_, err := fmt.Fprintf(cmd.OutOrStdout(), "no checkpoints in %s\n", list.DB)
		return err
	}
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "BUCKET\tV\tCOVERS W\tMINTED AT\tVALUE")
	for _, r := range list.Checkpoints {
		_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\n", r.SpanID, r.V, r.CoversW, r.MintedAt, r.ValueHash)
	}
	return w.Flush()
}

// mintDeps assembles the mint's dependencies from config. The ledger is opened
// read-only: minting writes nothing to it. The operational store takes the checkpoint
// rows and the artifact directory takes the values. The returned func closes all three.
func mintDeps(ctx context.Context, cfg config.Config) (mint.Deps, func(), error) {
	buckets, err := cfg.Buckets()
	if err != nil {
		return mint.Deps{}, nil, err
	}
	store, err := artifact.New(cfg.ArtifactDir)
	if err != nil {
		return mint.Deps{}, nil, err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		return mint.Deps{}, nil, err
	}
	l, err := openLedgerReadOnly(ctx, cfg)
	if err != nil {
		_ = db.Close()
		return mint.Deps{}, nil, err
	}
	closeAll := func() {
		_ = l.Close()
		_ = db.Close()
		_ = store.Close()
	}
	return mint.Deps{
		Ledger: l, Index: db, Artifacts: store, Buckets: buckets, Clock: clock.Real{}, Budget: cfg.CheckpointBudget,
	}, closeAll, nil
}
