package app

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/capture/bpcsv"
	"github.com/mattjoyce/anam/internal/capture/documents"
	"github.com/mattjoyce/anam/internal/capture/healthdb"
	"github.com/mattjoyce/anam/internal/capture/records"
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
		healthdb.AlertType,
	},
	bpcsv.Source:    {bpcsv.Type},
	withings.Source: {withings.Type},
	// records carries what is written down about the person: lab reports, the
	// self-log, notes made from email. Each type has a Definition whose identity is
	// the record's own id (docs/SELF-LOG.md). The one exception is blood pressure,
	// whose Definition composes identity from the reading's time and ordinal, so a
	// diary record's id is exactly that composition.
	records.Source: {
		"lab.result", "lab.panel", "condition.noted", "donation.blood", "health.weight",
		"med.missed-dose", "med.started", "med.dose-changed", "med.stopped", "med.compliance-estimate",
		"illness.started", "illness.ended", "illness.hangover",
		"travel.departed", "travel.arrived", "travel.returned", "travel.flight",
		"visit.clinic", "gym.visit", bpcsv.Type,
	},
	// documents is a folder of files whose bytes stay there: each becomes a
	// doc.reference whose payload is the path, the hash and the sidecar's date.
	documents.Source: {documents.Type},
}

func initCmd(cfgPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "idempotently create the ledger file and the operational store",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()

			l, err := ledger.Create(ctx, cfg.LedgerPath, cfg.PersonID)
			if err != nil {
				return err
			}
			defer func() { _ = l.Close() }()
			buckets, err := cfg.Buckets()
			if err != nil {
				return err
			}
			db, err := ops.Open(ctx, cfg.DBPath, buckets)
			if err != nil {
				return err
			}
			defer func() { _ = db.Close() }()

			_, err = fmt.Fprintf(cmd.OutOrStdout(),
				"ledger %s ready (person %s); operational store %s ready\n",
				cfg.LedgerPath, cfg.PersonID, cfg.DBPath)
			return err
		},
	}
}

// openLedgerForWrite opens an existing ledger for writing. It creates nothing: ingest
// against a path with no ledger fails and names `anam init`.
func openLedgerForWrite(ctx context.Context, cfg config.Config) (*ledger.Ledger, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	return ledger.Open(ctx, cfg.LedgerPath, cfg.PersonID)
}

func ingestCmd(cfgPath *string) *cobra.Command {
	var importsPath string
	batch := gate.MaxBatch

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
with the shell history; a declared import is reviewable and repeatable.

Every import is read before any is admitted. A source that cannot be read stops
the run with nothing written.

With a server configured the daemon runs its own declared imports, and this waits
for the run to finish, up to ingest_timeout. A source, --imports and --batch name
local files and are refused there.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if batch < 1 || batch > gate.MaxBatch {
				return fmt.Errorf("--batch %d is outside 1..%d", batch, gate.MaxBatch)
			}
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			if cfg.Server != "" {
				if len(args) > 0 || cmd.Flags().Changed("imports") || cmd.Flags().Changed("batch") {
					return fmt.Errorf("a source, --imports and --batch name local files; "+
						"the daemon at %s runs the imports it declares", cfg.Server)
				}
				return remoteIngest(cmd, cfg)
			}
			imports, err := loadImports(importsPath, cmd)
			if err != nil {
				return err
			}
			imports = withLibrary(imports, cfg)
			if len(args) == 1 {
				imports = filterImports(imports, args[0])
				if len(imports) == 0 {
					return fmt.Errorf("no import declared for source %q", args[0])
				}
			}

			return runIngest(cmd, cfg, imports, batch)
		},
	}
	cmd.Flags().StringVar(&importsPath, "imports", "",
		"imports file (default: $ANAM_IMPORTS, then <user config dir>/anam/imports.yaml)")
	cmd.Flags().IntVar(&batch, "batch", gate.MaxBatch,
		"facets per transaction; 1 is one commit per facet (max "+strconv.Itoa(gate.MaxBatch)+")")
	return cmd
}

// scanned is one declared import's envelopes, held until every import has been read.
type scanned struct {
	source string
	envs   []facet.Envelope
}

// runIngest reads every import, then admits them in declared order.
//
// The ledger is opened only after the last scan succeeds. A source that cannot be
// read, such as a file on a share that is not mounted, ends the run with the ledger
// never opened for writing.
func runIngest(cmd *cobra.Command, cfg config.Config, imports []config.Import, batch int) error {
	reg, err := def.Bundled()
	if err != nil {
		return err
	}
	if err := guardDefinitions(cmd.Context(), cfg, reg); err != nil {
		return err
	}
	all, err := scanAll(cmd.Context(), imports, cfg)
	if err != nil {
		return err
	}

	l, err := openLedgerForWrite(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	g := gate.New(l, reg, mvpScopes, clock.Real{}, cfg.PersonID)

	// The report is printed on both paths, and before the error: the writes that
	// landed are in the ledger whatever the exit code says, and an operator deciding
	// whether to re-run needs to know there were any.
	changed, err := admitSources(cmd.Context(), g, all, cfg.Timeout, batch, func(sc scanned, prog progress) {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), sourceLine(resultOf(sc.source, len(sc.envs), prog)))
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(cmd.OutOrStdout(), verdict(changed))
	return err
}

// remoteIngest asks the daemon for an ingest, waits for it, and reports it as a local
// run does: a line per source, then the verdict or the error.
func remoteIngest(cmd *cobra.Command, cfg config.Config) error {
	c, err := newClient(cfg)
	if err != nil {
		return err
	}
	op, err := c.Ingest(cmd.Context(), cfg.IngestTimeout)
	if err != nil {
		return err
	}
	for _, r := range op.Sources {
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), sourceLine(r))
	}
	switch op.State {
	case api.StateSucceeded:
		_, err = fmt.Fprintln(cmd.OutOrStdout(), op.Summary)
		return err
	case api.StateFailed:
		return fmt.Errorf("ingest %s on %s: %s", op.ID, c.Server(), op.Error)
	default:
		return fmt.Errorf("ingest %s on %s is still %s after %s; anam status shows it",
			op.ID, c.Server(), op.State, cfg.IngestTimeout)
	}
}

// admitSources admits each scanned import in declared order and calls each with its
// progress once it is done, or once it fails, before the error is returned. It
// stops at the first failure and returns how many facets were admitted or
// superseded before it.
func admitSources(ctx context.Context, g admitter, all []scanned, timeout time.Duration, batch int,
	each func(scanned, progress),
) (int, error) {
	changed := 0
	for _, sc := range all {
		prog, err := admitAll(ctx, g, sc.envs, timeout, batch)
		each(sc, prog)
		changed += prog.tally[gate.OutcomeAdmitted] + prog.tally[gate.OutcomeSuperseded]
		if err != nil {
			return changed, err
		}
	}
	return changed, nil
}

// nothingNew is the last line of a complete run that wrote nothing. A scheduler
// matches on it to stay quiet about such a run; api.Verdict owns the wording, so
// the CLI and the API say the same thing.
const nothingNew = api.NothingNew

// verdict is the last line of a complete run.
func verdict(changed int) string { return api.Verdict(changed) }

// scanAll reads every declared import before any of them is admitted, and stops at the
// first that cannot be read. Its error names the declared source and says nothing was
// admitted, because the reader of a failed scheduled run needs both.
func scanAll(ctx context.Context, imports []config.Import, cfg config.Config) ([]scanned, error) {
	defs, err := def.Bundled()
	if err != nil {
		return nil, err
	}
	all := make([]scanned, 0, len(imports))
	for _, imp := range imports {
		envs, err := scanImport(ctx, imp, cfg, defs)
		if err != nil {
			return nil, fmt.Errorf("import %s could not be read, nothing admitted: %w", imp.Source, err)
		}
		all = append(all, scanned{source: imp.Source, envs: envs})
	}
	return all, nil
}

// scanImport dispatches one declared import to its capture worker.
func scanImport(ctx context.Context, imp config.Import, cfg config.Config, defs *def.Registry) ([]facet.Envelope, error) {
	switch imp.Source {
	case healthdb.Source:
		scanCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		return healthdb.Scan(scanCtx, imp.Path, cfg.PersonID)
	case bpcsv.Source:
		return bounded(ctx, func() ([]facet.Envelope, error) { return bpcsv.Scan(imp.Path, cfg.PersonID) })
	case withings.Source:
		scanCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
		return withings.Scan(scanCtx, imp.Path, cfg.PersonID)
	case records.Source:
		return bounded(ctx, func() ([]facet.Envelope, error) { return records.Scan(imp.Path, cfg.PersonID, defs) })
	case documents.Source:
		return bounded(ctx, func() ([]facet.Envelope, error) { return documents.Scan(imp.Path, cfg.PersonID) })
	default:
		return nil, fmt.Errorf("import names unknown source %q (have: %s, %s, %s, %s, %s)",
			imp.Source, healthdb.Source, bpcsv.Source, withings.Source, records.Source, documents.Source)
	}
}

// bounded runs a scan that takes no context and returns when ctx ends, if that is
// first. The file scanners read plain files and cannot be interrupted, but a read on
// a share whose server has gone can block for as long as it likes, and the run must
// still end at its deadline. The abandoned scan finishes or fails on its own; its
// result is discarded and it holds nothing the run needs.
func bounded(ctx context.Context, scan func() ([]facet.Envelope, error)) ([]facet.Envelope, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	type result struct {
		envs []facet.Envelope
		err  error
	}
	done := make(chan result, 1)
	go func() {
		envs, err := scan()
		done <- result{envs, err}
	}()
	select {
	case r := <-done:
		return r.envs, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("scan abandoned: %w", ctx.Err())
	}
}

// withLibrary adds the default library as a documents import when nothing declared
// one and the folder exists. The library needs no declaration because its place is
// derived from the ledger's; a declared documents import names another folder and
// wins. A library that is not there yet is empty, which is a state, not a fault.
func withLibrary(imports []config.Import, cfg config.Config) []config.Import {
	for _, imp := range imports {
		if imp.Source == documents.Source {
			return imports
		}
	}
	if info, err := os.Stat(cfg.LibraryDir); err != nil || !info.IsDir() {
		return imports
	}
	return append(imports, config.Import{Source: documents.Source, Path: cfg.LibraryDir})
}

// guardDefinitions checks the bundled Definitions against the ones the operational
// store last saw (def.Guard) and stores the current set. It runs before any source is
// read: a Definition that would re-key or re-place the ledger's facets must stop the
// run before the gate exists, and the message is the remedy.
func guardDefinitions(ctx context.Context, cfg config.Config, reg *def.Registry) error {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	buckets, err := cfg.Buckets()
	if err != nil {
		return err
	}
	db, err := ops.Open(ctx, cfg.DBPath, buckets)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return guardWith(ctx, db, reg)
}

// guardWith is guardDefinitions on an operational store the caller already holds,
// which is how the daemon holds it.
func guardWith(ctx context.Context, db *ops.DB, reg *def.Registry) error {
	rows, err := db.Definitions(ctx)
	if err != nil {
		return err
	}
	stored := make([]def.Stored, len(rows))
	for i, r := range rows {
		stored[i] = def.Stored{Type: facet.Type(r.Type), Version: r.Version, Body: []byte(r.Body)}
	}
	if err := def.Guard(stored, reg); err != nil {
		return err
	}
	current, err := def.Manifests()
	if err != nil {
		return err
	}
	put := make([]ops.DefinitionRow, len(current))
	for i, m := range current {
		put[i] = ops.DefinitionRow{Type: string(m.Type), Version: m.Version, Body: string(m.Body)}
	}
	return db.PutDefinitions(ctx, put)
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

// admitter is the commit surface admitAll needs, and nothing more: resolve a binding,
// then commit a block of envelopes under it (interfaces at the point of use,
// AGENTS.md). *gate.Gate satisfies it.
type admitter interface {
	Resolve(env facet.Envelope) (facet.Binding, error)
	CommitBatch(ctx context.Context, envs []facet.Envelope, bindings []facet.Binding) ([]gate.Result, ledger.Watermark, error)
}

// progress is what an ingest is known to have done: the outcomes counted so far and
// the watermark the last completed block left behind.
//
// It exists because the interesting case is the failed one. A block that committed is
// in the ledger whether or not a later block fails, so "how far did it get" is an
// answer the operator is owed on the error path — and held in loop-local variables it
// was exactly what the error path stepped over.
type progress struct {
	tally map[gate.Outcome]int
	w     ledger.Watermark

	// done is how many envelopes have an outcome. It is also what says whether w
	// means anything: zero blocks completed leaves w at the zero watermark, which is
	// a real value meaning "empty ledger" and must not be reported as one.
	done int
}

// resultOf is one source's part of a run as the API reports it, whichever process ran
// it.
func resultOf(source string, offered int, p progress) api.SourceResult {
	r := api.SourceResult{
		Source:     source,
		Offered:    offered,
		Admitted:   p.tally[gate.OutcomeAdmitted],
		Superseded: p.tally[gate.OutcomeSuperseded],
		Unchanged:  p.tally[gate.OutcomeNoOp],
		Committed:  p.done,
	}
	if p.done > 0 {
		w := int64(p.w)
		r.W = &w
	}
	return r
}

// sourceLine renders one source's part of a run, naming itself partial when it is.
func sourceLine(r api.SourceResult) string {
	if r.Committed == 0 || r.W == nil {
		return fmt.Sprintf("%s: %d envelopes → nothing committed", r.Source, r.Offered)
	}
	s := fmt.Sprintf("%s: %d envelopes → admitted=%d superseded=%d no-op=%d  W=%d",
		r.Source, r.Offered, r.Admitted, r.Superseded, r.Unchanged, *r.W)
	if r.Committed < r.Offered {
		s += fmt.Sprintf("  (stopped after %d of %d — commit is idempotent, so re-run)", r.Committed, r.Offered)
	}
	return s
}

// admitAll drives every envelope through resolve → commit in blocks of `batch`,
// each block under its own deadline. It stops at the first error: commit is
// idempotent, so a failed ingest is re-run rather than resumed.
//
// It returns what it managed alongside the error rather than instead of it. Re-running
// is always safe, but "safe to re-run" is not "nothing happened": dying at block 2 and
// dying at block 100 want different responses from the operator, and only the count
// tells them apart.
//
// The block size is throughput, not semantics. A block of 1 is exactly the old
// one-transaction-per-facet path; larger blocks amortise the three sequential round
// trips a commit costs over many facets, at the price of retrying more work when a
// watermark goes stale. For a single-writer backfill nothing contends and the price
// is never paid.
func admitAll(ctx context.Context, g admitter, envs []facet.Envelope, timeout time.Duration, batch int,
) (progress, error) {
	prog := progress{tally: map[gate.Outcome]int{}}

	for start := 0; start < len(envs); start += batch {
		end := min(start+batch, len(envs))
		chunk := envs[start:end]

		bindings := make([]facet.Binding, len(chunk))
		for i, env := range chunk {
			b, err := g.Resolve(env)
			if err != nil {
				return prog, fmt.Errorf("resolve %s: %w", env.SourceRef, err)
			}
			bindings[i] = b
		}

		// The timeout bounds one call, so a batch gets proportionally longer than a
		// single commit: it is one round trip's latency times three, not per facet,
		// but the transaction itself is larger.
		callCtx, cancel := context.WithTimeout(ctx, timeout)
		results, w, err := g.CommitBatch(callCtx, chunk, bindings)
		cancel()
		if err != nil {
			return prog, fmt.Errorf("commit batch at %d: %w", start, err)
		}
		for _, res := range results {
			prog.tally[res.Outcome]++
		}
		prog.w = w
		prog.done += len(results)
	}
	return prog, nil
}
