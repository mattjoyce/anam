package app

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/capture/healthdb"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// mvpScopes is the MVP's one static source → types token map (§6, authenticated
// pillar). bp-csv joins when its worker and definition exist.
var mvpScopes = gate.StaticScopes{
	healthdb.Source: {
		"health.sleep", "health.body-composition", "health.heart-rate", "health.metric-group",
	},
}

func initCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "idempotently create the ledger table, seed its watermark, and create the operational schema",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()

			l, err := ledger.New(ctx, cfg.DDBEndpoint, cfg.DDBRegion, cfg.TableName, cfg.PersonID)
			if err != nil {
				return err
			}
			if err := l.Init(ctx); err != nil {
				return err
			}
			db, err := ops.Open(ctx, cfg.DBPath)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"ledger table %q ready (person %s); operational store %s ready\n",
				cfg.TableName, cfg.PersonID, cfg.DBPath)
			return err
		},
	}
}

func ingestCmd(cfgPath *string) *cobra.Command {
	var dbPath string

	cmd := &cobra.Command{
		Use:   "ingest health-db --db <path>",
		Short: "capture a source and admit every row through the gate",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if args[0] != healthdb.Source {
				return fmt.Errorf("unknown source %q (have: %s)", args[0], healthdb.Source)
			}

			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			reg, err := def.Bundled()
			if err != nil {
				return err
			}
			l, err := ledger.New(cmd.Context(), cfg.DDBEndpoint, cfg.DDBRegion, cfg.TableName, cfg.PersonID)
			if err != nil {
				return err
			}
			g := gate.New(l, reg, mvpScopes, clock.Real{}, cfg.PersonID)

			scanCtx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			envs, err := healthdb.Scan(scanCtx, dbPath, cfg.PersonID)
			cancel()
			if err != nil {
				return err
			}

			tally, w, err := admitAll(cmd.Context(), g, envs, cfg.Timeout)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"%s: %d envelopes → admitted=%d superseded=%d no-op=%d  W=%d\n",
				healthdb.Source, len(envs),
				tally[gate.OutcomeAdmitted], tally[gate.OutcomeSuperseded], tally[gate.OutcomeNoOp], w)
			return err
		},
	}
	cmd.Flags().StringVar(&dbPath, "db", "", "path to the source database (required)")
	_ = cmd.MarkFlagRequired("db")
	return cmd
}

// admitAll drives every envelope through resolve → commit, each under its own
// deadline. It stops at the first error: the batch is re-runnable to known-good
// (commit is idempotent), so a failed ingest is re-run, not resumed.
func admitAll(ctx context.Context, g *gate.Gate, envs []facet.Envelope, timeout time.Duration,
) (map[gate.Outcome]int, ledger.Watermark, error) {
	tally := map[gate.Outcome]int{}
	var w ledger.Watermark
	for _, env := range envs {
		binding, err := g.Resolve(env)
		if err != nil {
			return nil, 0, fmt.Errorf("resolve %s: %w", env.SourceRef, err)
		}
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		res, err := g.Commit(callCtx, env, binding)
		cancel()
		if err != nil {
			return nil, 0, fmt.Errorf("commit %s: %w", env.SourceRef, err)
		}
		tally[res.Outcome]++
		w = res.W
	}
	return tally, w, nil
}
