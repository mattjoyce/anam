package api

import (
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/receipt"
)

// FacetShown is one facet as a receipt served it (card 91), beside the links the
// record holds to it (card 104). The entry is the receipt's served value at the
// receipt's W, the payload and envelope as admitted, which its content hash covers;
// the links are a derived part read from the edge index at their own watermark, and
// are never written into the facet.
type FacetShown struct {
	Hash    string          `json:"hash"`
	Receipt receipt.Receipt `json:"receipt"`
	Entry   fold.Entry      `json:"entry"`
	Links   Links           `json:"links"`
}

// Links is the live edges touching one facet, at the watermark W the edge index
// had replayed to when they were read. Edges is empty, never absent, when none touch
// it.
type Links struct {
	W     int64  `json:"w"`
	Edges []Link `json:"edges"`
}

// Link is one live edge facet touching the facet shown: its id, its kind, the
// moment it was asserted (its fact-time), which side the facet shown is on (from,
// to, or both), and every facet id on each of its sides.
type Link struct {
	Edge string   `json:"edge"`
	Kind string   `json:"kind"`
	At   string   `json:"at"`
	Side string   `json:"side"`
	From []string `json:"from"`
	To   []string `json:"to"`
}
