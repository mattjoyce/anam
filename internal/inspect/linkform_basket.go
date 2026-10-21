package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/store/ledger"
)

// The link form's basket (card 202): the facets a link names, read from the link being
// edited or from the selection page's handoff, and described from the ledger.

// edgeValue is a stored record.edge payload, as the form reads it to edit.
type edgeValue struct {
	ID         string            `json:"id"`
	At         string            `json:"at"`
	SourceTime json.RawMessage   `json:"source_time"`
	Kind       string            `json:"kind"`
	From       []string          `json:"from"`
	To         []string          `json:"to"`
	About      []string          `json:"about"`
	Pins       map[string]string `json:"pins"`
	Text       string            `json:"text"`
}

// editableEdge is the link being edited, read from its current version: a live
// record.edge the Workbench entered, as the entry form corrects only its own facets.
// It answers the request itself and returns false otherwise.
func (s *Server) editableEdge(ctx context.Context, w http.ResponseWriter, id string) (edgeValue, bool) {
	f, ok := s.correctable(ctx, w, catchup.EdgeType, id)
	if !ok {
		return edgeValue{}, false
	}
	var v edgeValue
	if err := json.Unmarshal(f.Payload.Bytes(), &v); err != nil {
		httpError(w, fmt.Errorf("inspect: link %s: %w", id, err))
		return edgeValue{}, false
	}
	if len(v.To) == 0 {
		v.To = v.About // record.note v1's name of to
	}
	if v.Kind == "" {
		v.Kind = "about"
	}
	return v, true
}

// fillFrom sets the form from a stored link: its kind, words, moment and zone, and
// its sides and pins as the basket.
func (lf *linkForm) fillFrom(d *linkData, v edgeValue) {
	d.Kind, d.Text = v.Kind, v.Text
	fld := formField{Name: "at", Pointer: lf.at.timeAt, Input: "datetime-local"}
	if raw, err := json.Marshal(v.At); err == nil {
		d.At = lf.at.textOf(&fld, raw)
	}
	var st struct{ Zone, Offset string }
	if json.Unmarshal(v.SourceTime, &st) == nil {
		d.Zone, d.Offset = st.Zone, st.Offset
	}
	d.Items = nil
	for _, id := range v.From {
		d.Items = append(d.Items, linkItem{ID: id, From: true, Keep: true, Pin: v.Pins[id] != "", Hash: v.Pins[id]})
	}
	for _, id := range v.To {
		d.Items = append(d.Items, linkItem{ID: id, Keep: true, Pin: v.Pins[id] != "", Hash: v.Pins[id]})
	}
	sortItems(d.Items)
}

// basketOf is the facet ids a handoff names: what its program picks, read through the
// projection it was built over, and its ticks. The program is resolved here, once, so
// the form carries ids and the link names facets, not a query. problem is the
// evaluator's or the projection's refusal, in its words, with no ids.
func (s *Server) basketOf(ctx context.Context, q url.Values) (ids []string, problem string, err error) {
	set := index(values(q["tick"]))
	if prog := strings.TrimSpace(q.Get("program")); prog != "" {
		proj := q.Get("p")
		if proj == "" {
			proj = canonicalName
		}
		served, res, err := s.servedFor(ctx, proj)
		if errors.Is(err, errUnknownProjection) {
			return nil, err.Error(), nil
		}
		if err != nil {
			return nil, "", err
		}
		defs, err := s.definitions()
		if err != nil {
			return nil, "", err
		}
		ans, _, err := read.Evaluate(ctx, served, res.Live, defs, prog, selection.Relation, s.maxReadBytes)
		if err != nil {
			return nil, "The selection could not be read, so the form has no facets from it: " + err.Error(), nil
		}
		for _, t := range ans.Tuples {
			set[t[0].Text()] = true
		}
	}
	return setOf(set), "", nil
}

// itemsOf is the basket as items, the from side first, each side by id, pinned as
// pins says or, for a facet it does not name, as the kind pins by default.
func itemsOf(ids []string, from map[string]bool, pins map[string]string, kind string) []linkItem {
	items := make([]linkItem, 0, len(ids))
	for _, id := range ids {
		it := linkItem{ID: id, From: from[id], Keep: true, Pin: !unpinnedByDefault[kind]}
		if h, ok := pins[id]; ok {
			it.Pin, it.Hash = true, h
		}
		items = append(items, it)
	}
	sortItems(items)
	return items
}

func sortItems(items []linkItem) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].From != items[j].From {
			return items[i].From
		}
		return items[i].ID < items[j].ID
	})
}

// describe reads each item's current version through the ledger: its type and fact
// time for the page, and its content hash, which a pin names unless the item already
// names one. An id the ledger does not hold is drawn as such.
func (s *Server) describe(ctx context.Context, d *linkData) error {
	for i := range d.Items {
		it := &d.Items[i]
		f, err := s.ledger.CurrentVersion(ctx, it.ID)
		if errors.Is(err, ledger.ErrNotFound) {
			it.Held = false
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect: facet %s: %w", it.ID, err)
		}
		it.Held, it.Type, it.FactTime, it.Current = true, string(f.Type), f.FactTime.Format(time.RFC3339), f.Descriptor.ContentHash
		if it.Hash == "" {
			it.Hash = it.Current
		}
	}
	return nil
}
