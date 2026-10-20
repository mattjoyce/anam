package inspect

import (
	"context"
	"fmt"

	"github.com/mattjoyce/anam/internal/api"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/store/ops"
)

// Edges is the narrow read surface of the edge index the facet page needs (card 104,
// slice 3): the live edges touching one facet, with the cursor they are current to.
// Interfaces at the point of use; the daemon's read-only *ops.DB satisfies it. Nil
// means there is no index to read, and the page says so rather than show none.
type Edges interface {
	TouchingEdges(ctx context.Context, facetID string) (ops.Touching, error)
}

// linkView is one link drawn: the API's Link as read.LinksOf answers it, beside the
// edge's own text read from the ledger by the edge's id, since the index holds ends
// and not words. The page draws the API's part and composes nothing of its own; the
// text is the edge facet's, shown here so a note reads beside what it is about.
type linkView struct {
	api.Link
	Text string
}

// linksData is the facet page's links section.
type linksData struct {
	// Note says why there is nothing to draw when no index was given; empty when
	// the index was read, however many edges it holds.
	Note string
	// W is the index's cursor when the edges were read: its own watermark, distinct
	// from the fold's, as in the API's answer.
	W     int64
	Links []linkView
}

// facetLinks is the links part of a facet's page, through the same code the API's
// GET /v1/receipts/{hash}/facets/{facet_id} answers from (read.LinksOf over the index),
// so the page and the API cannot drift. A wrong read refuses the page, as the API
// refuses its whole answer, rather than draw the facet with links absent.
func (s *Server) facetLinks(ctx context.Context, facetID string) (linksData, error) {
	if s.edges == nil {
		return linksData{Note: "no edge index to read: a standalone inspector over a bare ledger has none, the daemon's has"}, nil
	}
	t, err := s.edges.TouchingEdges(ctx, facetID)
	if err != nil {
		return linksData{}, err
	}
	links := read.LinksOf(t, facetID)
	out := linksData{W: links.W, Links: make([]linkView, 0, len(links.Edges))}
	for _, l := range links.Edges {
		text, err := s.edgeText(ctx, l.Edge)
		if err != nil {
			return linksData{}, err
		}
		out.Links = append(out.Links, linkView{Link: l, Text: text})
	}
	return out, nil
}

// edgeText is an edge's `text`, read from its current version in the ledger: for a
// note (kind about), the person's words. Empty when the edge carries none.
func (s *Server) edgeText(ctx context.Context, edgeID string) (string, error) {
	f, err := s.ledger.CurrentVersion(ctx, edgeID)
	if err != nil {
		return "", fmt.Errorf("inspect: edge %s: %w", edgeID, err)
	}
	doc, err := def.Decode(f.Payload.Bytes())
	if err != nil {
		return "", fmt.Errorf("inspect: edge %s payload: %w", edgeID, err)
	}
	v, err := def.Resolve(doc, "/text")
	if err != nil {
		return "", nil // an edge with no text: the index said it is live, the words are optional
	}
	text, _ := v.(string)
	return text, nil
}
