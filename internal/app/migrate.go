package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/migrate"
	"github.com/mattjoyce/anam/internal/safepath"
)

// A migration carries the live facets of one ledger into a fresh one through the gate
// (card 200): the old files are opened read-only and the new ones must not exist. It
// touches no daemon, so it is run with the old daemon stopped only so the facets it
// reads are the last ones; nothing it does can disturb a daemon that is running.

func migrateCmd(cfgPath *string) *cobra.Command {
	var o migrateOptions
	cmd := &cobra.Command{
		Use:   "migrate --from <ledger.db> --from-store <tracking.db> --to <ledger.db> --to-store <tracking.db> [--program <file>] [--yes]",
		Short: "carry the live facets of one ledger into a fresh ledger, through the gate",
		Long: `Migrate reads an old ledger and its operational store, read-only, and carries the
live facets of the ledger into a new ledger and operational store that must not exist.
Every facet arrives under its own type, source, source ref, binding and payload,
held to the Definition version it was admitted under, so its id, content hash and
fact time are the ones it had. Superseded versions and tombstoned facets are not
carried. Every registered Definition and projection of the old store is written to the
new one first, so every version a carried facet was admitted under is held.

With --program, only the facets a Datalog program's pick/1 selects are carried; the
program is run over the old ledger's whole live value, with no projection narrowing.
A record.edge that names a facet the selection does not carry is still carried, and
the report lists each such link.

Without --yes it is a dry run: it prints, per type and source, how many facets it would
carry and how many the gate would refuse, and writes nothing. With --yes it creates the
new files, carries the selection, then proves the new ledger's live facets equal the
selection and that a dry run against the new ledger would carry the same set. If any
facet would be refused, a real run writes nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := loadRoutedConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			return o.run(cmd, cfg)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.from, "from", "", "the old ledger file, opened read-only")
	f.StringVar(&o.fromStore, "from-store", "", "the old operational store file, opened read-only")
	f.StringVar(&o.to, "to", "", "the new ledger file; it must not exist")
	f.StringVar(&o.toStore, "to-store", "", "the new operational store file; it must not exist")
	f.StringVar(&o.program, "program", "", "a file holding a Datalog program that defines pick/1; default every live facet")
	f.BoolVar(&o.yes, "yes", false, "create the new files and carry the selection; without it the command only reports")
	for _, name := range []string{"from", "from-store", "to", "to-store"} {
		_ = cmd.MarkFlagRequired(name)
	}
	return cmd
}

type migrateOptions struct {
	from, fromStore, to, toStore, program string
	yes                                   bool
}

// run is the command past its flags: the paths settled, the dry run reported, and with
// --yes the carry and its proof.
func (o migrateOptions) run(cmd *cobra.Command, cfg config.Config) error {
	src, to, err := o.paths(cfg)
	if err != nil {
		return err
	}
	if err := to.CheckAbsent(); err != nil {
		return err
	}
	program, err := o.programText()
	if err != nil {
		return err
	}
	p := migrate.Params{
		Person: cfg.PersonID, Timeout: cfg.Timeout, Phase: cfg.LongWriteTimeout,
		Limits: renameLimits(cfg),
	}
	out := cmd.OutOrStdout()

	start := time.Now()
	plan, err := migrate.Read(cmd.Context(), src, program, p)
	if err != nil {
		return err
	}
	took := []step{{"read the old ledger, select, ask the gate", time.Since(start)}}
	writeMigratePlan(out, plan, program != "")
	if n := plan.Refused(); n > 0 {
		return fmt.Errorf("%d facets the gate would refuse; nothing written", n)
	}
	if !o.yes {
		_, err = fmt.Fprintf(out, "dry run: nothing written. Re-run with --yes to carry %d facets.\n", len(plan.Items))
		return err
	}
	return o.carry(cmd.Context(), out, plan, src, to, p, program, took)
}

// paths settles the four flags: the old files must exist and the new ones are made.
func (o migrateOptions) paths(cfg config.Config) (migrate.Source, migrate.Target, error) {
	var src migrate.Source
	var to migrate.Target
	var err error
	if src.Ledger, err = safepath.ResolveFile(o.from); err != nil {
		return src, to, fmt.Errorf("--from: %w", err)
	}
	if src.Store, err = safepath.ResolveFile(o.fromStore); err != nil {
		return src, to, fmt.Errorf("--from-store: %w", err)
	}
	if to.Ledger, err = safepath.Expand(o.to); err != nil {
		return src, to, fmt.Errorf("--to: %w", err)
	}
	if to.Store, err = safepath.Expand(o.toStore); err != nil {
		return src, to, fmt.Errorf("--to-store: %w", err)
	}
	if to.Buckets, err = cfg.Buckets(); err != nil {
		return src, to, err
	}
	return src, to, nil
}

// programText is the program file's text, or empty for every live facet.
func (o migrateOptions) programText() (string, error) {
	if o.program == "" {
		return "", nil
	}
	fh, err := safepath.Open(o.program)
	if err != nil {
		return "", fmt.Errorf("--program: %w", err)
	}
	defer func() { _ = fh.Close() }()
	body, err := io.ReadAll(fh)
	if err != nil {
		return "", fmt.Errorf("--program %s: %w", o.program, err)
	}
	if len(body) == 0 {
		return "", fmt.Errorf("--program %s is empty", o.program)
	}
	return string(body), nil
}

// step is one timed phase of a run.
type step struct {
	name string
	took time.Duration
}

// carry is the real run: the new files made and filled, then the proof that the new
// ledger holds the selection and that a second plan, read from it, is the same.
func (o migrateOptions) carry(ctx context.Context, out io.Writer, plan migrate.Plan, src migrate.Source,
	to migrate.Target, p migrate.Params, program string, took []step,
) error {
	start := time.Now()
	done, err := migrate.Carry(ctx, plan, to, p)
	if err != nil {
		return err
	}
	took = append(took, step{"create the new files and carry", time.Since(start)})
	if _, err := fmt.Fprintf(out, "carried %d facets into %s; the new ledger is at W %d\n", done.Admitted, to.Ledger, done.W); err != nil {
		return err
	}

	start = time.Now()
	proof, err := migrate.Prove(ctx, plan, to.Ledger, p)
	if err != nil {
		return err
	}
	took = append(took, step{"prove the new ledger equals the selection", time.Since(start)})
	writeProof(out, plan, proof, src, to)
	if !proof.Equal() {
		return fmt.Errorf("the new ledger does not equal the selection: %d differences; keep the old files and do not switch", len(proof.Mismatches))
	}

	start = time.Now()
	again, err := migrate.Read(ctx, migrate.Source{Ledger: to.Ledger, Store: to.Store}, program, p)
	if err != nil {
		return err
	}
	took = append(took, step{"a second dry run against the new ledger", time.Since(start)})
	if err := migrate.Same(plan, again); err != nil {
		return fmt.Errorf("the carry is not idempotent: %w", err)
	}
	_, _ = fmt.Fprintf(out, "idempotent: a dry run with the new ledger as --from carries the same %d facets, ids and content hashes equal\n", len(again.Items))
	writeSteps(out, took)
	return nil
}

// writeMigratePlan is the dry run's report: the head, the count to carry and to
// refuse per type and source, what the gate would refuse and why, the links that would
// dangle, and the ids a program picked that no live facet holds.
func writeMigratePlan(out io.Writer, plan migrate.Plan, program bool) {
	how := "every live facet"
	if program {
		how = "the facets the program picks"
	}
	_, _ = fmt.Fprintf(out, "old ledger at W %d: %d live facets; selection is %s: %d facets\n", plan.W, plan.Live, how, len(plan.Items))
	type key struct {
		typ    facet.Type
		source string
	}
	carry, refused := map[key]int{}, map[key]int{}
	var keys []key
	for _, it := range plan.Items {
		k := key{it.Facet.Type, it.Facet.Provenance.Source}
		if _, seen := carry[k]; !seen {
			if _, seen := refused[k]; !seen {
				keys = append(keys, k)
			}
		}
		if it.Fate == migrate.FateCarry {
			carry[k]++
		} else {
			refused[k]++
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].typ != keys[j].typ {
			return keys[i].typ < keys[j].typ
		}
		return keys[i].source < keys[j].source
	})
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TYPE\tSOURCE\tCARRY\tREFUSED")
	totalCarry, totalRefused := 0, 0
	for _, k := range keys {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", k.typ, k.source, carry[k], refused[k])
		totalCarry += carry[k]
		totalRefused += refused[k]
	}
	_, _ = fmt.Fprintf(w, "total\t\t%d\t%d\n", totalCarry, totalRefused)
	_ = w.Flush()
	writeRefusals(out, plan)
	for _, l := range plan.Links {
		_, _ = fmt.Fprintf(out, "link to a facet not carried: %s (%s) %s %s, %s\n",
			l.Edge.Provenance.SourceRef, l.Edge.ID, l.Side, l.FacetID, l.Standing)
	}
	for _, id := range plan.NotLive {
		_, _ = fmt.Fprintf(out, "picked but not live, not carried: %s\n", id)
	}
}

// writeRefusals names why the gate would refuse, grouped by type, source and Definition
// version with one example each: a refusal repeats across a type's thousands of facets.
func writeRefusals(out io.Writer, plan migrate.Plan) {
	type key struct {
		typ     facet.Type
		source  string
		version int
	}
	type group struct {
		n       int
		example string
		reason  string
	}
	groups := map[key]*group{}
	var keys []key
	for _, it := range plan.Items {
		if it.Fate != migrate.FateRefused {
			continue
		}
		k := key{it.Facet.Type, it.Facet.Provenance.Source, it.Facet.DefinitionVersion}
		g := groups[k]
		if g == nil {
			g = &group{example: it.Facet.Provenance.SourceRef, reason: it.Reason}
			groups[k] = g
			keys = append(keys, k)
		}
		g.n++
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].typ != keys[j].typ {
			return keys[i].typ < keys[j].typ
		}
		if keys[i].source != keys[j].source {
			return keys[i].source < keys[j].source
		}
		return keys[i].version < keys[j].version
	})
	for _, k := range keys {
		g := groups[k]
		_, _ = fmt.Fprintf(out, "refused %d %s from %s at v%d, for example %s: %s\n", g.n, k.typ, k.source, k.version, g.example, g.reason)
	}
}

// writeProof prints the head of each ledger, the sizes of the old files and the new,
// and whether the new ledger's live facets equal the selection.
func writeProof(out io.Writer, plan migrate.Plan, proof migrate.Proof, src migrate.Source, to migrate.Target) {
	_, _ = fmt.Fprintf(out, "W old %d, W new %d\n", plan.W, proof.W)
	for _, f := range []struct{ name, old, new string }{{"ledger", src.Ledger, to.Ledger}, {"operational store", src.Store, to.Store}} {
		_, _ = fmt.Fprintf(out, "%s: old %s, new %s\n", f.name, sizeOf(f.old), sizeOf(f.new))
	}
	if proof.Equal() {
		_, _ = fmt.Fprintf(out, "proof: the new ledger's %d live facets equal the selection's %d, facet for facet by id, content hash, fact time and status\n",
			proof.Live, len(plan.Items))
		return
	}
	_, _ = fmt.Fprintf(out, "proof FAILED: the new ledger holds %d live facets, the selection %d\n", proof.Live, len(plan.Items))
	for _, m := range proof.Mismatches {
		_, _ = fmt.Fprintf(out, "  %s\n", m)
	}
}

// sizeOf is a file's size in bytes, or why it has none.
func sizeOf(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "unknown (" + err.Error() + ")"
	}
	return fmt.Sprintf("%d bytes", info.Size())
}

func writeSteps(out io.Writer, took []step) {
	for _, s := range took {
		_, _ = fmt.Fprintf(out, "took %s: %s\n", s.took.Round(time.Millisecond), s.name)
	}
}
