package read

import (
	"context"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/fold"
)

// Coverage counts what the record holds at the W q names, type by type and source by
// source (card 90). It is a lens over the same view the timeline reads, at the head,
// at q.W or at q.AsOf, through q.Projection or not, so it carries that read's W and hash and
// the same W gives the same answer. It is counted from the value rather than the
// operational index because the index stands only at its own cursor: a past W needs
// the fold anyway, and one path gives one answer.
func Coverage(ctx context.Context, src Source, q api.CoverageQuery) (api.Coverage, error) {
	// Only as_of reads a zone, so a query without one needs none.
	var loc *time.Location
	if q.AsOf != "" {
		var err error
		if loc, err = Location(q.Zone); err != nil {
			return api.Coverage{}, err
		}
	}
	know, err := knowledgeOf(api.TimelineQuery{W: q.W, AsOf: q.AsOf}, loc)
	if err != nil {
		return api.Coverage{}, err
	}
	v, err := readView(ctx, src, know, q.Projection, "")
	if err != nil {
		return api.Coverage{}, err
	}
	id, err := src.Ledger.ID(ctx)
	if err != nil {
		return api.Coverage{}, err
	}
	servers, err := projectionsServing(src)
	if err != nil {
		return api.Coverage{}, err
	}
	types := coverageOf(v.shown.Entries())
	for i := range types {
		// Never nil, so the field reads [] and not null: an empty list is the gap.
		types[i].Projections = append([]string{}, servers[types[i].Type]...)
	}
	t := v.describe()
	return api.Coverage{
		W: t.W, LedgerID: id, Live: v.shown.Len(), Canonical: t.Canonical, Projection: t.Projection,
		Types: types,
	}, nil
}

// projectionsServing maps each type to the sorted names of the projections that serve
// it, the newest version of each name the source holds, resolved against the Definitions
// held by Covers. A type in no list is one no projection can reach, so the query route
// cannot answer it (card 231). It is the same for every W: the templates and the
// Definitions are the ones in force now, not the ones at the W counted.
func projectionsServing(src Source) (map[string][]string, error) {
	reg, err := src.projections()
	if err != nil {
		return nil, err
	}
	defs, err := src.definitions()
	if err != nil {
		return nil, err
	}
	held := defs.All()
	servers := map[string][]string{}
	for _, tpl := range reg.All() { // in name order, so each list is sorted
		for _, typ := range tpl.Covers(held) {
			servers[string(typ)] = append(servers[string(typ)], tpl.Name)
		}
	}
	return servers, nil
}

// coverageOf groups entries by type and, within a type, by source, sorted by name.
// Fact-times are fixed-width clock strings, so the earliest and latest are the least
// and greatest strings.
func coverageOf(entries []fold.Entry) []api.TypeCoverage {
	types := map[string]*api.TypeCoverage{}
	sources := map[string]map[string]*api.SourceCoverage{}
	for _, e := range entries {
		typ := string(e.Type)
		tc, ok := types[typ]
		if !ok {
			tc = &api.TypeCoverage{Type: typ, Earliest: e.FactTime, Latest: e.FactTime}
			types[typ], sources[typ] = tc, map[string]*api.SourceCoverage{}
		}
		tc.Count++
		tc.Earliest, tc.Latest = min(tc.Earliest, e.FactTime), max(tc.Latest, e.FactTime)
		sc, ok := sources[typ][e.Source]
		if !ok {
			sc = &api.SourceCoverage{Source: e.Source, Earliest: e.FactTime, Latest: e.FactTime}
			sources[typ][e.Source] = sc
		}
		sc.Count++
		sc.Earliest, sc.Latest = min(sc.Earliest, e.FactTime), max(sc.Latest, e.FactTime)
	}
	out := make([]api.TypeCoverage, 0, len(types))
	for typ, tc := range types {
		for _, sc := range sources[typ] {
			tc.Sources = append(tc.Sources, *sc)
		}
		sort.Slice(tc.Sources, func(i, j int) bool { return tc.Sources[i].Source < tc.Sources[j].Source })
		out = append(out, *tc)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}
