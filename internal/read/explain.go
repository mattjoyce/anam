package read

import (
	"context"
	"sort"

	"github.com/mattjoyce/anam/internal/api"
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

// explain fills t with the verdict on every artifact version the read stands on,
// narrowed by fact-time and type. The counts are taken before the narrowing.
func explain(ctx context.Context, l fold.Ledger, v view, b bounds, typeFilter string, t api.Timeline,
) (api.Timeline, error) {
	rows, counts, err := explainRows(ctx, l, v)
	if err != nil {
		return api.Timeline{}, err
	}
	for _, r := range rows {
		if typeFilter != "" && r.Type != typeFilter {
			continue
		}
		if !b.admits(fold.Entry{FactTime: r.FactTime}) {
			continue
		}
		t.Verdicts = append(t.Verdicts, r)
	}
	t.Shown = len(t.Verdicts)
	t.Counts = api.VerdictCounts{
		Served: counts["served"], Excluded: counts["excluded"], Superseded: counts["superseded"],
	}
	return t, nil
}

// explainRows builds one row per artifact version that has ever been in force for a
// facet the fold knows about, at the view's watermark.
func explainRows(ctx context.Context, l fold.Ledger, v view) ([]api.VerdictRow, map[string]int, error) {
	counts := map[string]int{}

	// The live versions, with the template's verdict on each. Without a projection
	// every live facet stands served — the canonical is the un-narrowed read.
	var rows []api.VerdictRow
	if v.tmpl != nil {
		verdicts, err := projection.Explain(v.res.Value, *v.tmpl, consent.Open{}, v.defs)
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

	// One order for every row, the canonical's own: fact-time then id, with the
	// version breaking ties so a facet's history reads oldest-first in place.
	sortRows(rows)
	return rows, counts, nil
}

func rowOfEntry(e fold.Entry, version int, standing, reason string) api.VerdictRow {
	return api.VerdictRow{
		FactTime:  e.FactTime,
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
func rowOfDisplacement(d fold.Displacement) api.VerdictRow {
	return api.VerdictRow{
		FactTime:  clock.Format(d.Facet.FactTime),
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
func sortRows(rows []api.VerdictRow) {
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		switch {
		case a.FactTime != b.FactTime:
			return a.FactTime < b.FactTime
		case a.FacetID != b.FacetID:
			return a.FacetID < b.FacetID
		default:
			return a.Version < b.Version
		}
	})
}
