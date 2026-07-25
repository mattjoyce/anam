package app

import (
	"context"
	"fmt"
	"sort"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/projection"
)

// The explain view answers the question a governed system has to be able to answer:
// at this point in Anam's own clock, which facts does this projection stand on, and
// which have been displaced?
//
// Three kinds of row, and the distinction between them is the whole point:
//
//	served       the version in force, and the projection includes it
//	excluded     the version in force, and the projection excludes it — with the
//	             reason, from the same verdict Apply used
//	superseded   a version that was in force once and is not now
//
// Superseded rows are not in the canonical and never reach a template — the canonical
// is live-only by construction (§9). They are derived from the fold: a facet folded to
// v3 has had v1 and v2 displaced, which is knowable without asking the ledger anything
// extra. The artifacts are then fetched so the row can show what was displaced rather
// than merely that something was.

// verdictRow is one artifact version and its standing.
type verdictRow struct {
	EventTime string
	Type      string
	Version   int
	Standing  string
	Reason    string
	SourceRef string
	FacetID   string
}

func writeExplain(cmd *cobra.Command, ctx context.Context, l fold.Ledger, v view, b bounds, typeFilter string) error {
	rows, counts, err := explainRows(ctx, l, v)
	if err != nil {
		return err
	}

	shown := rows[:0:0]
	for _, r := range rows {
		if typeFilter != "" && r.Type != typeFilter {
			continue
		}
		if !b.admits(fold.Entry{EventTime: r.EventTime}) {
			continue
		}
		shown = append(shown, r)
	}

	out := cmd.OutOrStdout()
	_, _ = fmt.Fprint(out, v.header(len(shown)))
	_, _ = fmt.Fprintf(out, "verdicts   %d served, %d excluded, %d superseded\n",
		counts["served"], counts["excluded"], counts["superseded"])
	if v.tmpl == nil {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
			"\nno --projection: every live facet is 'served' by definition; the reasons appear once a template is named")
	}

	w := tabwriter.NewWriter(out, 2, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "EVENT TIME\tTYPE\tV\tSTANDING\tWHY\tSOURCE REF")
	for _, r := range shown {
		_, _ = fmt.Fprintf(w, "%s\t%s\tv%d\t%s\t%s\t%s\n",
			r.EventTime, r.Type, r.Version, r.Standing, r.Reason, r.SourceRef)
	}
	return w.Flush()
}

// explainRows builds one row per artifact version that has ever been in force for a
// facet the fold knows about, at the view's watermark.
func explainRows(ctx context.Context, l fold.Ledger, v view) ([]verdictRow, map[string]int, error) {
	counts := map[string]int{}

	// The live versions, with the template's verdict on each. Without a projection
	// every live facet stands served — the canonical is the un-narrowed read.
	var rows []verdictRow
	if v.tmpl != nil {
		verdicts, err := projection.Explain(v.res.Value, *v.tmpl, consent.Open{})
		if err != nil {
			return nil, nil, err
		}
		for _, x := range verdicts {
			standing, reason := "served", ""
			if !x.Reason.Served() {
				standing, reason = "excluded", string(x.Reason)
				if x.Detail != "" {
					reason += ": " + x.Detail
				}
			}
			counts[standing]++
			rows = append(rows, rowOfEntry(x.Entry, v.res.States[x.Entry.FacetID].Version, standing, reason))
		}
	} else {
		for _, e := range v.res.Value.Entries() {
			counts["served"]++
			rows = append(rows, rowOfEntry(e, v.res.States[e.FacetID].Version, "served", ""))
		}
	}

	displaced, err := fold.Displaced(ctx, l, v.res)
	if err != nil {
		return nil, nil, err
	}
	counts["superseded"] = len(displaced)
	for _, d := range displaced {
		rows = append(rows, rowOfDisplacement(d))
	}

	// One order for every row, the canonical's own: event-time then id, with the
	// version breaking ties so a facet's history reads oldest-first in place.
	sortRows(rows)
	return rows, counts, nil
}

func rowOfEntry(e fold.Entry, version int, standing, reason string) verdictRow {
	return verdictRow{
		EventTime: e.EventTime,
		Type:      string(e.Type),
		Version:   version,
		Standing:  standing,
		Reason:    reason,
		SourceRef: e.SourceRef,
		FacetID:   e.FacetID,
	}
}

// rowOfDisplacement renders a version that is not in force. Every such row stands
// "superseded" — the word covers displaced-by-a-correction and no-longer-live alike,
// and fold.Displaced's reason says which.
func rowOfDisplacement(d fold.Displacement) verdictRow {
	return verdictRow{
		EventTime: clock.Format(d.Facet.EventTime),
		Type:      string(d.Facet.Type),
		Version:   d.Version,
		Standing:  "superseded",
		Reason:    d.Reason,
		SourceRef: d.Facet.Provenance.SourceRef,
		FacetID:   d.Facet.ID,
	}
}

// sortRows imposes the canonical's order — insertion order is arbitrary because
// States is a map — with the version breaking ties so a facet's history reads
// oldest-first in place.
func sortRows(rows []verdictRow) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch {
		case a.EventTime != b.EventTime:
			return a.EventTime < b.EventTime
		case a.FacetID != b.FacetID:
			return a.FacetID < b.FacetID
		default:
			return a.Version < b.Version
		}
	})
}
