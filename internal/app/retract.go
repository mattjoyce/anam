package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/config"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/gate"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/store/ledger"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Admission admits and supersedes; it never retracts, because a row's absence from a
// batch and a source's absence look alike from inside one envelope. Retraction is
// therefore its own command, in two parts that stay apart: the report is a set
// comparison (live facets of the source, minus the ids the source holds now), and
// the tombstones are a deliberate step the operator asks for (card 17). What the
// source holds now is its importer's newest complete snapshot (card 78).
//
// Two guards stand between a bad read and a mass retraction. A snapshot that
// carried nothing is refused outright: a source that forgot everything is
// indistinguishable from one that was not there, and no one means to tombstone a
// whole source by accident. Beyond that, a fraction bounds how much of a source may
// go in one run, and the operator raises it on purpose.

func retractCmd(cfgPath *string) *cobra.Command {
	var yes, retired bool
	var maxFraction float64
	var facetID string

	cmd := &cobra.Command{
		Use:   "retract <source>",
		Short: "tombstone the facets a source no longer holds, or one facet by hand",
		Long: `Retract compares what the ledger holds live from one source with what the
source holds now, and lists the facets the source has dropped. Nothing is
written unless --yes is given; then each listed facet is tombstoned through
the gate, one anam-event each, and the watermark moves. The artifacts stay in
the ledger, marked.

What the source holds now is its importer's newest complete snapshot: the plan
is the live facets of the snapshot's types that the run did not carry and that
nothing has touched since it completed. An importer never tombstones; this
command, with --yes, does.

A snapshot that carried nothing is refused: an empty source and an unreadable
one look the same, and neither is a reason to forget a record. --max-fraction
bounds how much of a source may be retracted in one run; raise it deliberately
when the source really has shrunk that far.

A source replaced by an importer under another name is retired: --retired plans
every live facet of the source, by type, and tombstones them all with --yes. It
refuses a source an importer still declares, and asks for --max-fraction 1, so
the whole source goes only on purpose. The artifacts stay; a read --at a time
before the retirement still shows them.

A source nobody resends (workbench, conversation) has no snapshot to compare with.
There a wrong entry is undone one facet at a time: --facet <id> shows the facet and
what the tombstone would do, and with --yes tombstones that one facet. It refuses a
facet that belongs to another source, a facet that is not live, and a source an
importer declares and has sent a complete snapshot for, which its next resend
reconciles instead. The artifact stays; a read --at before it still shows the facet.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source := args[0]
			cfg, err := loadConfig(*cfgPath, cmd)
			if err != nil {
				return err
			}
			if facetID != "" {
				if retired {
					return errors.New("--facet and --retired are two different retractions; pass one")
				}
				return runFacetRetraction(cmd, cfg, source, facetID, yes)
			}
			if retired {
				return runRetirement(cmd, cfg, source, yes, maxFraction)
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), cfg.Timeout)
			defer cancel()

			absent, total, heldN, err := planFromSnapshot(ctx, cmd, cfg, source)
			if err != nil {
				return err
			}
			writeRetractionReport(cmd, source, absent, total, heldN)
			if len(absent) == 0 {
				return nil
			}
			if err := guardRetraction(len(absent), total, heldN, maxFraction); err != nil {
				return err
			}
			if !yes {
				_, err = fmt.Fprintln(cmd.OutOrStdout(), "dry run: nothing written. Re-run with --yes to tombstone them.")
				return err
			}
			return retractAll(cmd, cfg, absent)
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "write the tombstones; without it the command only reports")
	cmd.Flags().Float64Var(&maxFraction, "max-fraction", 0.1,
		"refuse to retract more than this fraction of the source's live facets in one run")
	cmd.Flags().StringVar(&facetID, "facet", "",
		"retract this one facet of the source by hand (for a source with no importer snapshot)")
	cmd.Flags().BoolVar(&retired, "retired", false,
		"the source is retired, not rescanned: plan every live facet it holds (needs --max-fraction 1 to write)")
	return cmd
}

// planRetraction is the report as a value: the live entries of the source whose ids
// the source no longer holds, in timeline order, and how many live facets the
// source has in all.
func planRetraction(live []fold.Entry, source string, held map[string]bool) (absent []fold.Entry, total int) {
	for _, e := range live {
		if e.Source != source {
			continue
		}
		total++
		if !held[e.FacetID] {
			absent = append(absent, e)
		}
	}
	// fact_time is fixed-width clock.Layout text, so the lexical order is the
	// chronological one.
	sort.Slice(absent, func(i, j int) bool {
		if absent[i].FactTime != absent[j].FactTime {
			return absent[i].FactTime < absent[j].FactTime
		}
		return absent[i].FacetID < absent[j].FacetID
	})
	return absent, total
}

// snapshotStore is what planning from a snapshot asks of the operational store: the
// newest complete resend of a source and the facet ids it carried. *ops.DB satisfies
// it; the interface is here, where it is used, so the plan names what it needs.
type snapshotStore interface {
	NewestCompleteSnapshot(ctx context.Context, source string) (ops.Snapshot, bool, error)
	SnapshotFacets(ctx context.Context, id string) (map[string]bool, error)
}

// planLedger is what facetsOf asks of the ledger.
type planLedger interface {
	LatestVersions(ctx context.Context, ids []string) (map[string]int, error)
	Versions(ctx context.Context, refs []ledger.VersionRef) ([]facet.Facet, error)
}

// facetsOf fetches each planned entry's facet at its newest version, in the plan's
// order. A live facet's newest version is the one in force, unless a batch
// superseded it since the read; then the report shows the newer one, and the gate,
// which tombstones whatever version is current, is unaffected.
func facetsOf(ctx context.Context, l planLedger, entries []fold.Entry) ([]facet.Facet, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.FacetID
	}
	latest, err := l.LatestVersions(ctx, ids)
	if err != nil {
		return nil, err
	}
	refs := make([]ledger.VersionRef, len(ids))
	for i, id := range ids {
		refs[i] = ledger.VersionRef{FacetID: id, Version: latest[id]}
	}
	got, err := l.Versions(ctx, refs)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]facet.Facet, len(got))
	for _, f := range got {
		byID[f.ID] = f
	}
	out := make([]facet.Facet, len(ids))
	for i, id := range ids {
		out[i] = byID[id]
	}
	return out, nil
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
	cat, err := loadCatalog(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	reg := cat.Definitions()
	l, err := openLedgerForWrite(cmd.Context(), cfg)
	if err != nil {
		return err
	}
	defer func() { _ = l.Close() }()
	// A tombstone admits nothing, so the gate needs no scopes for it.
	g := gate.New(l, reg, gate.StaticScopes{}, clock.Real{}, cfg.PersonID, gate.Limits{MaxPayloadBytes: cfg.MaxPayloadBytes, MaxRetries: cfg.CommitRetries})

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

// planFromSnapshot plans an importer's source from its newest complete snapshot: the
// live facets of the snapshot's types that the run did not carry and that nothing has
// touched since it completed (read.Absent, the same answer GET /v1/snapshots/{id}
// gives). held is how many facets the run carried, for the guard against an empty
// "now". With no complete snapshot there is nothing to plan from, which is an error,
// not an empty plan.
func planFromSnapshot(ctx context.Context, cmd *cobra.Command, cfg config.Config, source string,
) (absent []facet.Facet, total, held int, err error) {
	src, closeAll, err := readSource(ctx, cfg)
	if err != nil {
		return nil, 0, 0, err
	}
	defer closeAll()
	db, ok := src.Checkpoints.(snapshotStore)
	if !ok {
		return nil, 0, 0, fmt.Errorf("retract: planning from a snapshot needs the operational store's snapshot log (NewestCompleteSnapshot, SnapshotFacets); the read source holds a %T", src.Checkpoints)
	}
	snap, ok, err := db.NewestCompleteSnapshot(ctx, source)
	if err != nil {
		return nil, 0, 0, err
	}
	if !ok {
		return nil, 0, 0, fmt.Errorf("source %q has sent no complete snapshot to plan from; a source replaced by another is retired with --retired", source)
	}
	carried, err := db.SnapshotFacets(ctx, snap.ID)
	if err != nil {
		return nil, 0, 0, err
	}
	hist, ok := src.Ledger.(read.FacetHistory)
	if !ok {
		return nil, 0, 0, fmt.Errorf("retract: judging absence needs a ledger that answers one facet's events (FacetEvents); the read source holds a %T", src.Ledger)
	}
	entries, live, err := read.Absent(ctx, src, hist, read.SnapshotScope{
		Source: source, Types: snap.Types, Carried: carried, StartedW: ledger.Watermark(snap.StartedW),
	})
	if err != nil {
		return nil, 0, 0, err
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: planned from snapshot %s (%s), begun at W=%d, completed %s at W=%d\n",
		source, snap.ID, strings.Join(snap.Types, ", "), snap.StartedW, clock.Format(snap.CompletedAt), snap.CompletedW)
	facets, err := facetsOf(ctx, src.Ledger, entries)
	return facets, live, len(carried), err
}
