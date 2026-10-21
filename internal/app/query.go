package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
	"github.com/mattjoyce/anam/internal/query"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

func queryCmd(cfgPath *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query",
		Short: "the Datalog read over a served value",
		Long: `Query is the third read: a Datalog program evaluated over what a projection serves,
answered with a receipt. describe says what can be asked, run asks it,
and check holds the bundled rules to the fold over a real ledger.`,
	}
	cmd.AddCommand(queryDescribeCmd(cfgPath), queryRunCmd(cfgPath), queryCheckCmd(cfgPath))
	return cmd
}

func queryCheckCmd(cfgPath *string) *cobra.Command {
	var ledgerPath, person, projName string
	var every int
	cmd := &cobra.Command{
		Use:   "check",
		Short: "hold the bundled current/2 equal to fold.Snapshot over a real ledger",
		Long: `Check is the equality check the query surface stands on: Anam's bundled Datalog
rule current(E, At) must keep exactly what fold.Snapshot keeps, at every instant.
The tests hold it on fixtures; this holds it on a record.

It opens the ledger read-only, folds it at the head, serves the log through
--projection when one is named (the whole canonical otherwise), and compares the
two at every --every-th distinct fact-time, one instant per program. It writes
nothing and moves no watermark. The first disagreement ends the run with the ids
each side holds; otherwise it reports how many instants agreed and how long each
took.

--ledger and --person point it at a copy of a record rather than the configured
one; the bundled Definitions and projections are used either way.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := checkConfig(*cfgPath, cmd, ledgerPath, person)
			if err != nil {
				return err
			}
			return runQueryCheck(cmd, cfg, projName, every)
		},
	}
	cmd.Flags().StringVar(&ledgerPath, "ledger", "", "ledger file to check (default: the configured ledger_path)")
	cmd.Flags().StringVar(&person, "person", "", "the person the ledger is pinned to (default: the configured person_id)")
	cmd.Flags().StringVar(&projName, "projection", "", "a log-shaped projection to serve through (default: the whole canonical)")
	cmd.Flags().IntVar(&every, "every", 1, "compare every Nth distinct fact-time; 1 compares them all")
	return cmd
}

// checkConfig is the command's config, with --ledger and --person overriding the
// file the way --server does, so a copy of a record can be checked from a machine
// with no config of its own. With a configured server and no --ledger the daemon owns
// the configured files, and the command refuses as every local command does.
func checkConfig(explicit string, cmd *cobra.Command, ledgerPath, person string) (config.Config, error) {
	path, origin, err := config.Resolve(explicit)
	if err != nil {
		return config.Config{}, err
	}
	set := map[string]string{}
	if f := cmd.Flag("server"); f != nil && f.Changed {
		set["server"] = f.Value.String()
	}
	if ledgerPath != "" {
		set["ledger_path"] = ledgerPath
	}
	if person != "" {
		set["person_id"] = person
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
	if cfg.Server != "" && ledgerPath == "" {
		return config.Config{}, fmt.Errorf("%s opens local files, and with server %s configured the daemon owns them; "+
			"pass --ledger to check a copy, or stop the daemon and unset server (ANAM_SERVER, --server)",
			cmd.CommandPath(), cfg.Server)
	}
	return cfg, nil
}

func runQueryCheck(cmd *cobra.Command, cfg config.Config, projName string, every int) error {
	ctx := cmd.Context()
	defs, err := def.Bundled()
	if err != nil {
		return err
	}
	l, err := openLedgerReadOnly(ctx, cfg)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()

	served, res, header, err := checkValue(ctx, l, defs, projName)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprint(cmd.OutOrStdout(), header)
	report, err := query.Check(ctx, served, query.Versions(res.Live), defs, every)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "compared    %d of %d fact-times (every %d)  %.2fs total  %.2fms mean  slowest %.2fms at %s\n",
		report.Instants, report.FactTimes, every, report.Elapsed.Seconds(),
		meanMillis(report), report.Slowest.Seconds()*1e3, report.SlowestAt)
	if d := report.Disagreement; d != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "DISAGREE at %s\n  current/2      %d: %s\n  fold.Snapshot  %d: %s\n",
			d.At, len(d.Current), strings.Join(d.Current, " "), len(d.Snapshot), strings.Join(d.Snapshot, " "))
		return fmt.Errorf("current/2 and fold.Snapshot disagree at %s", d.At)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "OK   current/2 = fold.Snapshot at every instant compared\n")
	return nil
}

// checkValue folds the ledger at its head and serves it as the value to check: the
// whole canonical, or the log a named projection serves. A snapshot-shaped
// projection has already taken its latest per series, so there would be nothing
// left to compare; it is refused.
func checkValue(ctx context.Context, l *ledger.Ledger, defs *def.Registry, projName string) (fold.Value, fold.Result, string, error) {
	id, err := l.ID(ctx)
	if err != nil {
		return fold.Value{}, fold.Result{}, "", err
	}
	res, err := fold.Read(ctx, l)
	if err != nil {
		return fold.Value{}, fold.Result{}, "", err
	}
	header := fmt.Sprintf("ledger      %s  W=%d  live=%d\n", id, res.W, res.Value.Len())
	if projName == "" {
		return res.Value, res, header, nil
	}
	templates, err := projection.Bundled()
	if err != nil {
		return fold.Value{}, fold.Result{}, "", err
	}
	t, err := templates.Lookup(projName)
	if err != nil {
		return fold.Value{}, fold.Result{}, "", err
	}
	if t.Shape != projection.ShapeLog {
		return fold.Value{}, fold.Result{}, "", fmt.Errorf("projection %s is %s-shaped and has already taken its latest per series; name a log-shaped one", t.Name, t.Shape)
	}
	served, err := serve.Derive(res.Value, t, consent.Open{}, defs, "")
	if err != nil {
		return fold.Value{}, fold.Result{}, "", err
	}
	header += fmt.Sprintf("projection  %s v%d  %s  served=%d\n", t.Name, t.Version, t.Hash, served.Len())
	return served, res, header, nil
}

func meanMillis(r query.Report) float64 {
	if r.Instants == 0 {
		return 0
	}
	return r.Elapsed.Seconds() * 1e3 / float64(r.Instants)
}
