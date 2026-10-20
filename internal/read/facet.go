package read

import (
	"errors"
	"fmt"
	"slices"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/serve"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// ErrNotServed is returned by Facet for a facet the receipt did not serve: a facet
// id alone opens nothing a projection did not serve (card 91).
var ErrNotServed = errors.New("read: the receipt did not serve that facet")

// Facet reads one facet as the receipt served it, from the artifacts alone, with no
// links yet: the entry is the receipt's value, and the caller joins the links the
// index holds at its own watermark (LinksOf).
func Facet(d serve.Deps, hash, facetID string) (api.FacetShown, error) {
	r, shown, err := serve.Show(d, hash)
	if err != nil {
		return api.FacetShown{}, err
	}
	if len(shown.Entries) == 0 {
		return api.FacetShown{}, fmt.Errorf("%w: receipt %s served a frame or a query answer, not entries", ErrNotServed, hash)
	}
	for _, e := range shown.Entries {
		if e.FacetID == facetID {
			return api.FacetShown{Hash: hash, Receipt: r, Entry: e, Links: api.Links{Edges: []api.Link{}}}, nil
		}
	}
	return api.FacetShown{}, fmt.Errorf("%w: %s is not among the %d entries receipt %s served", ErrNotServed, facetID, len(shown.Entries), hash)
}

// LinksOf is the links part of a facet's answer: the edges touching facetID as the
// index read them, each saying which side the facet is on.
func LinksOf(t ops.Touching, facetID string) api.Links {
	links := api.Links{W: t.LastSeq, Edges: make([]api.Link, 0, len(t.Edges))}
	for _, e := range t.Edges {
		side := ""
		switch from, to := slices.Contains(e.From, facetID), slices.Contains(e.To, facetID); {
		case from && to:
			side = "both"
		case from:
			side = ops.SideFrom
		default:
			side = ops.SideTo
		}
		links.Edges = append(links.Edges, api.Link{
			Edge: e.EdgeID, Kind: e.Kind, At: e.FactTime, Side: side,
			From: orEmpty(e.From), To: orEmpty(e.To),
		})
	}
	return links
}

func orEmpty(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}
