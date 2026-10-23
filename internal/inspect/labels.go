package inspect

import (
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// Labels (card 199, slice 3): a page that draws a facet as a thing reads its label from
// the Definition in force for the facet's type (def.Definition.Label). A type whose
// Definition declares none has no label, and the page keeps the type and the time.

// labelClip is how many characters of a label a row shows. It is presentation, like a
// page size: a longer label is cut with an ellipsis, and the facet's page has the rest.
const labelClip = 80

// labeler renders labels from a registry of Definitions.
type labeler struct{ reg *def.Registry }

// of is the label of a payload of type t, or "" when the type has none, clipped to
// labelClip characters.
func (l labeler) of(t facet.Type, payload []byte) string {
	if l.reg == nil {
		return ""
	}
	d, ok := l.reg.Lookup(t)
	if !ok {
		return ""
	}
	s := d.Label(payload)
	if r := []rune(s); len(r) > labelClip {
		return string(r[:labelClip-1]) + "…"
	}
	return s
}

// labels reads the label of each facet a served value holds, by id.
type labels struct {
	l       labeler
	entries map[string]fold.Entry
}

// newLabels is the labels of the facets in served, under the Definitions in reg.
func newLabels(reg *def.Registry, served fold.Value) labels {
	m := make(map[string]fold.Entry, served.Len())
	for _, e := range served.Entries() {
		m[e.FacetID] = e
	}
	return labels{l: labeler{reg}, entries: m}
}

// of is the label of the facet with this id, "" when it is not in the value or its
// type declares none.
func (ls labels) of(id string) string {
	e, ok := ls.entries[id]
	if !ok {
		return ""
	}
	return ls.l.of(e.Type, e.Payload)
}
