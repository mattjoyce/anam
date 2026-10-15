package app

import (
	"context"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/read"
)

// A retired source is neither empty nor unreadable: it is gone by decision, replaced
// by an importer under another name, and nothing will scan it again. Retract's plan
// is a set comparison, the live facets of a source minus what the source holds now;
// retirement is the same plan with "holds now" declared empty by the operator rather
// than read from a snapshot. Two guards replace the empty-snapshot refusal: no
// importer may still name the source, and the operator must ask for
// the whole fraction. Each tombstone is then one anam-event through the gate, as for
// any retraction (card 120).

// runRetirement plans and, with yes, tombstones every live facet of a retired source.
func runRetirement(cmd *cobra.Command, cfg config.Config, source string, yes bool, maxFraction float64) error {
	if err := refuseDeclared(cmd, cfg, source); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
	defer cancel()
	live, err := planRetirement(ctx, cfg, source)
	if err != nil {
		return err
	}
	writeRetirementReport(cmd, source, live)
	if len(live) == 0 {
		return nil
	}
	if err := guardRetirement(source, len(live), maxFraction); err != nil {
		return err
	}
	if !yes {
		_, err = fmt.Fprintln(cmd.OutOrStdout(), "dry run: nothing written. Re-run with --yes to tombstone them.")
		return err
	}
	return retractAll(cmd, cfg, live)
}

// refuseDeclared refuses to retire a source that an importer still declares: a live
// source is retired by removing its declaration first, so the two steps cannot be
// confused. With no importers file configured there are no importers; that is said
// on stderr, so the operator sees what was checked.
func refuseDeclared(cmd *cobra.Command, cfg config.Config, source string) error {
	if cfg.ImportersFile == "" {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "importers: no importers file configured, so no importer declares the source")
		return nil
	}
	reg, err := def.Bundled()
	if err != nil {
		return err
	}
	importers, err := config.LoadPrincipals(cfg.ImportersFile, reg.Has)
	if err != nil {
		return err
	}
	for _, imp := range importers {
		if imp.Source == source {
			return fmt.Errorf("%s is still declared as an importer; remove it from %s before retiring it", source, cfg.ImportersFile)
		}
	}
	return nil
}

// planRetirement is every live facet of the source, at the version in force: the
// retraction plan with nothing held.
func planRetirement(ctx context.Context, cfg config.Config, source string) ([]facet.Facet, error) {
	src, closeAll, err := readSource(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer closeAll()
	res, err := read.Canonical(ctx, src)
	if err != nil {
		return nil, err
	}
	live, _ := planRetraction(res.Value.Entries(), source, nil)
	return facetsOf(ctx, src.Ledger, live)
}

// guardRetirement asks the operator to say, in the fraction, that the whole source
// goes. The default fraction refuses, so a retirement is never typed by habit.
func guardRetirement(source string, live int, maxFraction float64) error {
	if maxFraction < 1 {
		return fmt.Errorf("retiring %s tombstones all %d of its live facets; pass --max-fraction 1 to say so (given %.2f)",
			source, live, maxFraction)
	}
	return nil
}

// writeRetirementReport says what a retirement tombstones, by type. A source holds
// thousands of facets, so the report is the count per type, not a row per facet.
func writeRetirementReport(cmd *cobra.Command, source string, live []facet.Facet) {
	out := cmd.OutOrStdout()
	if len(live) == 0 {
		_, _ = fmt.Fprintf(out, "%s: retired; 0 live facets in the ledger, nothing to tombstone\n", source)
		return
	}
	_, _ = fmt.Fprintf(out, "%s: retired; %d live facets in the ledger, all to be tombstoned\n", source, len(live))
	byType := map[facet.Type]int{}
	for _, f := range live {
		byType[f.Type]++
	}
	types := make([]facet.Type, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "TYPE\tLIVE FACETS")
	for _, t := range types {
		_, _ = fmt.Fprintf(w, "%s\t%d\n", t, byType[t])
	}
	_ = w.Flush()
}
