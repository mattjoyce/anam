package app

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/mint"
	"github.com/mattjoyce/anam/internal/store/artifact"
	"github.com/mattjoyce/anam/internal/store/ops"
)

func checkpointCmd(cfgPath *string) *cobra.Command {
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "checkpoint",
		Short: "mint a checkpoint per bucket whose value has changed",
		Long: `Checkpoint folds the ledger at its head watermark, splits the live value by bucket,
and writes a new version of every bucket whose value differs from its last
checkpoint. A bucket that has not changed is left alone: same hash, no new row.

Each value is written to the artifact directory under the SHA-256 of its own bytes,
and the checkpoint log in the operational store records the bucket, its version and
the watermark it folded. Minting writes nothing to the ledger; a checkpoint is a
cache of the fold, there to bound replay, and is rebuilt by running this again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			deps, closeAll, err := mintDeps(cmd.Context(), cfg)
			if err != nil {
				return err
			}
			defer closeAll()

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()
			res, err := mint.Run(ctx, deps, dryRun)
			if err != nil {
				return err
			}

			out := cmd.OutOrStdout()
			w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "BUCKET\tV\tCOVERS W\tFACETS\tVALUE\t")
			for _, o := range res.Outcomes {
				note := ""
				switch {
				case o.Minted && !res.Written:
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
			if !res.Written {
				verb = "would mint (dry run — nothing written)"
			}
			_, err = fmt.Fprintf(out, "%s %d of %d buckets at W=%d\n",
				verb, res.Minted(), len(res.Outcomes), res.W)
			return err
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "derive and report, but publish nothing")
	cmd.AddCommand(checkpointListCmd(cfgPath))
	return cmd
}

func checkpointListCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list every checkpoint ever minted, by bucket and version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			buckets, err := cfg.Buckets()
			if err != nil {
				return err
			}
			// Listing reads the checkpoint log and nothing else, so it still answers
			// when there is no ledger to open.
			db, err := ops.Open(cmd.Context(), cfg.DBPath, buckets)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			rows, err := db.Checkpoints(cmd.Context())
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "no checkpoints in %s\n", cfg.DBPath)
				return err
			}
			w := tabwriter.NewWriter(cmd.OutOrStdout(), 2, 0, 2, ' ', 0)
			_, _ = fmt.Fprintln(w, "BUCKET\tV\tCOVERS W\tMINTED AT\tVALUE")
			for _, r := range rows {
				_, _ = fmt.Fprintf(w, "%s\t%d\t%d\t%s\t%s\n", r.SpanID, r.V, r.CoversW, r.MintedAt, r.ValueHash)
			}
			return w.Flush()
		},
	}
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
	return mint.Deps{Ledger: l, Index: db, Artifacts: store, Buckets: buckets, Clock: clock.Real{}}, closeAll, nil
}
