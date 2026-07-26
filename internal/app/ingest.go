package app

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/capture/bpcsv"
	"github.com/mattjoyce/anam/internal/capture/healthdb"
	"github.com/mattjoyce/anam/internal/capture/withings"
	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// mvpScopes is the MVP's one static source → types token map (§6, authenticated
// pillar).
var mvpScopes = gate.StaticScopes{
	healthdb.Source: {
		"health.sleep", "health.body-composition", "health.heart-rate", "health.metric-group",
	},
	bpcsv.Source:    {bpcsv.Type},
	withings.Source: {withings.Type},
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
	var importsPath string
	batch := ledger.MaxBatch

	cmd := &cobra.Command{
		Use:   "ingest [source]",
		Short: "run the declared imports through the gate",
		Long: `Ingest captures only what is declared. Imports are named in a YAML file —
--imports, then $ANAM_IMPORTS, then <user config dir>/anam/imports.yaml:

    imports:
      - source: health-db
        path: ~/admin/data/healthdata.db
      - source: bp-csv
        path: ~/exports/BloodPressure.csv

With no argument every declared import runs, in order; with a source name, the
declarations for that source. A path typed ad hoc is provenance that evaporates
with the shell history; a declared import is reviewable and repeatable.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if batch < 1 || batch > ledger.MaxBatch {
				return fmt.Errorf("--batch %d is outside 1..%d", batch, ledger.MaxBatch)
			}
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			imports, err := loadImports(importsPath, cmd)
			if err != nil {
				return err
			}
			if len(args) == 1 {
				imports = filterImports(imports, args[0])
				if len(imports) == 0 {
					return fmt.Errorf("no import declared for source %q", args[0])
				}
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

			for _, imp := range imports {
				envs, err := scanImport(cmd.Context(), imp, cfg)
				if err != nil {
					return err
				}
				tally, w, err := admitAll(cmd.Context(), g, envs, cfg.Timeout, batch)
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(),
					"%s: %d envelopes → admitted=%d superseded=%d no-op=%d  W=%d\n",
					imp.Source, len(envs),
					tally[gate.OutcomeAdmitted], tally[gate.OutcomeSuperseded], tally[gate.OutcomeNoOp], w)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&importsPath, "imports", "",
		"imports file (default: $ANAM_IMPORTS, then <user config dir>/anam/imports.yaml)")
	cmd.Flags().IntVar(&batch, "batch", ledger.MaxBatch,
		"facets per transaction; 1 restores one-commit-per-facet (max "+strconv.Itoa(ledger.MaxBatch)+")")
	return cmd
}

// scanImport dispatches one declared import to its capture worker.
func scanImport(ctx context.Context, imp config.Import, cfg config.Config) ([]facet.Envelope, error) {
	switch imp.Source {
	case healthdb.Source:
		scanCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		return healthdb.Scan(scanCtx, imp.Path, cfg.PersonID)
	case bpcsv.Source:
		return bpcsv.Scan(imp.Path, cfg.PersonID)
	case withings.Source:
		scanCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		return withings.Scan(scanCtx, imp.Path, cfg.PersonID)
	default:
		return nil, fmt.Errorf("import names unknown source %q (have: %s, %s, %s)",
			imp.Source, healthdb.Source, bpcsv.Source, withings.Source)
	}
}

func filterImports(imports []config.Import, source string) []config.Import {
	var out []config.Import
	for _, imp := range imports {
		if imp.Source == source {
			out = append(out, imp)
		}
	}
	return out
}

// loadImports resolves and loads the declarations, reporting provenance the same
// way loadConfig does.
func loadImports(explicit string, cmd *cobra.Command) ([]config.Import, error) {
	path, origin, err := config.ResolveImports(explicit)
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, fmt.Errorf("no imports file found — declare imports in %s or pass --imports",
			"<user config dir>/anam/imports.yaml")
	}
	_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "imports: %s (%s)\n", path, origin)
	return config.LoadImports(path)
}

// admitAll drives every envelope through resolve → commit in blocks of `batch`,
// each block under its own deadline. It stops at the first error: commit is
// idempotent, so a failed ingest is re-run rather than resumed.
//
// The block size is throughput, not semantics. A block of 1 is exactly the old
// one-transaction-per-facet path; larger blocks amortise the three sequential round
// trips a commit costs over many facets, at the price of retrying more work when a
// watermark goes stale. For a single-writer backfill nothing contends and the price
// is never paid.
func admitAll(ctx context.Context, g *gate.Gate, envs []facet.Envelope, timeout time.Duration, batch int,
) (map[gate.Outcome]int, ledger.Watermark, error) {
	tally := map[gate.Outcome]int{}
	var w ledger.Watermark

	for start := 0; start < len(envs); start += batch {
		end := min(start+batch, len(envs))
		chunk := envs[start:end]

		bindings := make([]facet.Binding, len(chunk))
		for i, env := range chunk {
			b, err := g.Resolve(env)
			if err != nil {
				return nil, 0, fmt.Errorf("resolve %s: %w", env.SourceRef, err)
			}
			bindings[i] = b
		}

		// The timeout bounds one call, so a batch gets proportionally longer than a
		// single commit: it is one round trip's latency times three, not per facet,
		// but the transaction itself is larger.
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		results, err := g.CommitBatch(callCtx, chunk, bindings)
		cancel()
		if err != nil {
			return nil, 0, fmt.Errorf("commit batch at %d: %w", start, err)
		}
		for _, res := range results {
			tally[res.Outcome]++
			w = res.W
		}
	}
	return tally, w, nil
}
