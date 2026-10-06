package app

import (
	"context"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
)

// Ingest admits and supersedes; it never retracts, because a row's absence from a
// scan and a source's absence look alike from inside one envelope. Retraction is
// therefore its own command, in two parts that stay apart: the report is a set
// comparison (live facets of the source, minus the ids the source holds now), and
// the tombstones are a deliberate step the operator asks for (card 17).
//
// Two guards stand between a bad read and a mass retraction. A scan that fails ends
// the run with nothing written, as it does for ingest. A scan that succeeds but
// holds nothing is refused outright: a source that forgot everything is
// indistinguishable from one that was not there, and no one means to tombstone a
// whole source by accident. Beyond that, a fraction bounds how much of a source may
// go in one run, and the operator raises it on purpose.

func retractCmd(cfgPath *string) *cobra.Command {
	var importsPath string
	var yes bool
	var maxFraction float64

	cmd := &cobra.Command{
		Use:   "retract <source>",
		Short: "tombstone the facets a source no longer holds",
		Long: `Retract compares what the ledger holds live from one source with what the
source holds now, and lists the facets the source has dropped. Nothing is
written unless --yes is given; then each listed facet is tombstoned through
the gate, one anam-event each, and the watermark moves. The artifacts stay in
the ledger, marked.

A scan that fails writes nothing. A scan that holds nothing is refused: an
empty source and an unreadable one look the same, and neither is a reason to
forget a record. --max-fraction bounds how much of a source may be retracted in
one run; raise it deliberately when the source really has shrunk that far.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			imports, err := loadImports(importsPath, cmd)
			if err != nil {
				return err
			}
			imports = filterImports(imports, source)
			if len(imports) == 0 {
				return fmt.Errorf("no import declares source %q", source)
			}
			scanned, err := scanAll(cmd.Context(), imports, cfg)
			if err != nil {
				return err
			}
			held := map[string]bool{}
			for _, sc := range scanned {
				for _, env := range sc.envs {
					held[facet.NewID(env.SourceRef)] = true
				}
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()
			l, err := openLedgerReadOnly(ctx, cfg)
			if err != nil {
				return err
			}
			res, err := fold.Read(ctx, l)
			_ = l.Close()
			if err != nil {
				return err
			}

			absent, total := planRetraction(res.Live, source, held)
			writeRetractionReport(cmd, source, absent, total, len(held))
			if len(absent) == 0 {
				return nil
			}
			if err := guardRetraction(len(absent), total, len(held), maxFraction); err != nil {
				return err
			}
			if !yes {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "dry run: nothing written. Re-run with --yes to tombstone them.")
				return err
			}
			return retractAll(cmd, cfg, absent)
		},
	}
	cmd.Flags().StringVar(&importsPath, "imports", "",
		"imports file (default: $ANAM_IMPORTS, then <user config dir>/anam/imports.yaml)")
	cmd.Flags().BoolVar(&yes, "yes", false, "write the tombstones; without it the command only reports")
	cmd.Flags().Float64Var(&maxFraction, "max-fraction", 0.1,
		"refuse to retract more than this fraction of the source's live facets in one run")
	return cmd
}

// planRetraction is the report as a value: the live facets of the source whose ids
// the source no longer holds, in timeline order, and how many live facets the
// source has in all.
func planRetraction(live []facet.Facet, source string, held map[string]bool) (absent []facet.Facet, total int) {
	for _, f := range live {
		if f.Provenance.Source != source {
			continue
		}
		total++
		if !held[f.ID] {
			absent = append(absent, f)
		}
	}
	sort.Slice(absent, func(i, j int) bool {
		if !absent[i].FactTime.Equal(absent[j].FactTime) {
			return absent[i].FactTime.Before(absent[j].FactTime)
		}
		return absent[i].ID < absent[j].ID
	})
	return absent, total
}

// guardRetraction refuses the two shapes of a bad read: a source that holds nothing,
// and a source that has lost more than the operator allowed.
func guardRetraction(absent, total, held int, maxFraction float64) error {
	if held == 0 {
		return fmt.Errorf("the source holds nothing, refusing to retract its %d live facets: an empty source and an unreadable one look the same", total)
	}
	if fraction := float64(absent) / float64(total); fraction > maxFraction {
		return fmt.Errorf("%d of %d live facets (%.0f%%) are no longer in the source, above --max-fraction %.2f; raise it if the source really shrank",
			absent, total, fraction*100, maxFraction)
	}
	return nil
}

func writeRetractionReport(cmd *cobra.Command, source string, absent []facet.Facet, total, held int) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "%s: %d live facets in the ledger, %d rows in the source, %d no longer held\n",
		source, total, held, len(absent))
	if len(absent) == 0 {
		return
	}
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "FACT TIME\tTYPE\tV\tSOURCE REF\tID")
	for _, f := range absent {
		_, _ = fmt.Fprintf(w, "%s\t%s\tv%d\t%s\t%s\n",
			clock.Format(f.FactTime), f.Type, f.ArtifactVersion, f.Provenance.SourceRef, f.ID)
	}
	_ = w.Flush()
}

// retractAll tombstones each facet through the gate, one event each. A crash
// mid-way leaves the ones done tombstoned and the rest live; a re-run re-plans and
// finishes, because a tombstoned facet is no longer live and drops out of the plan.
func retractAll(cmd *cobra.Command, cfg config.Config, absent []facet.Facet) error {
	reg, err := def.Bundled()
	if err != nil {
		return err
	}
	l, err := openLedgerForWrite(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	g := gate.New(l, reg, mvpScopes, clock.Real{}, cfg.PersonID)

	done := 0
	for _, f := range absent {
		ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
		res, _, err := g.Retract(ctx, f.ID)
		cancel()
		if err != nil {
			return fmt.Errorf("after %d of %d tombstoned, %s: %w (re-run to finish)", done, len(absent), f.Provenance.SourceRef, err)
		}
		if res.Outcome == gate.OutcomeTombstoned {
			done++
		}
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "tombstoned %d facets; the ledger moved\n", done)
	return err
}
