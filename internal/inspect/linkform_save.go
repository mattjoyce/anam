package inspect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/mattjoyce/anam/internal/catchup"
)

// edgePayload is the record.edge value the link form writes. Each side is sorted, so
// the same link saved twice is the same content and the gate's no-op.
type edgePayload struct {
	ID         string            `json:"id"`
	At         string            `json:"at"`
	SourceTime json.RawMessage   `json:"source_time,omitempty"`
	Kind       string            `json:"kind"`
	From       []string          `json:"from,omitempty"`
	To         []string          `json:"to,omitempty"`
	Pins       map[string]string `json:"pins,omitempty"`
	Text       string            `json:"text,omitempty"`
}

// posted reads the form back: every item it drew (item), the ones still in the link
// (keep), each one's side (side.<id>), the pinned ones (pin) and the version each pin
// names (hash.<id>), with the kind, the words, the moment and the confirmation.
func (lf linkForm) posted(vals url.Values) linkData {
	d := lf.page()
	d.Kind = strings.TrimSpace(vals.Get("kind"))
	d.Text = strings.TrimSpace(vals.Get("text"))
	d.At = strings.TrimSpace(vals.Get("at"))
	d.Zone, d.Offset = strings.TrimSpace(vals.Get(zoneField)), strings.TrimSpace(vals.Get(offsetField))
	d.Confirmed = vals.Get("confirm") == "yes"
	keep, pin := index(values(vals["keep"])), index(values(vals["pin"]))
	seen := map[string]bool{}
	for _, id := range values(vals["item"]) {
		if seen[id] {
			continue
		}
		seen[id] = true
		d.Items = append(d.Items, linkItem{
			ID: id, Keep: keep[id], From: vals.Get("side."+id) == "from", Pin: pin[id],
			Hash: strings.TrimSpace(vals.Get("hash." + id)),
		})
	}
	sortItems(d.Items)
	return d
}

// linkSubmit is POST /link: one record.edge, through the Workbench's entry, as source
// workbench. A new link's id is edgeID's; an edited one keeps the id it was saved
// with, so saving is a new version of it whatever changed. A form the kind's side
// rules refuse, a basket above confirmAbove not confirmed, a stale pin, a refusal or a
// busy queue draws the form again with what was posted, and writes nothing.
func (s *Server) linkSubmit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	version, err := strconv.Atoi(r.PostForm.Get("v"))
	if err != nil || version < 1 {
		http.Error(w, "v: want the Definition version the form was drawn from", http.StatusBadRequest)
		return
	}
	lf, ok := s.loadLinkForm(ctx, w)
	if !ok {
		return
	}
	data := lf.posted(r.PostForm)
	var stored edgeValue
	if edit := r.PostForm.Get("edit"); edit != "" {
		if stored, ok = s.editableEdge(ctx, w, edit); !ok {
			return
		}
		data.Edit = edit
	}
	if err := s.describe(ctx, &data); err != nil {
		httpError(w, err)
		return
	}
	payload, why := lf.payload(&data, r.PostForm, stored)
	if why != "" {
		data.Message = why
		renderStatus(w, http.StatusUnprocessableEntity, "link.html", data)
		return
	}
	// The version the form was drawn from goes back with the payload, so a Definition
	// that changed in between is refused by the gate, as for the entry form.
	res, err := s.bench.Enter(ctx, catchup.EdgeType, version, payload)
	var declined *Declined
	switch {
	case errors.As(err, &declined):
		data.Message = declined.Message
		if declined.Kind == DeclinedStale {
			data.Notice = fmt.Sprintf("The form below is drawn from the Definition now in force, v%d. "+
				"Check it, then save again.", data.Version)
		}
		renderStatus(w, declined.status(), "link.html", data)
	case err != nil:
		httpError(w, err)
	default:
		out := enteredPage(catchup.EdgeType, data.Edit, res)
		out.Links = []resultLink{
			{"/facet?id=" + url.QueryEscape(res.FacetID), "open the link"},
			{"/select", "start another link"},
		}
		renderStatus(w, http.StatusOK, "result.html", out)
	}
}

// payload builds the link from the posted form, or says why it will not: the kind
// off the Definition's list, a from side the kind does not take, more facets than
// confirmAbove without confirmation, no moment or offset, or a pin on a version that
// is no longer current. stored is the link being edited, whose id the payload keeps.
func (lf *linkForm) payload(d *linkData, vals url.Values, stored edgeValue) (json.RawMessage, string) {
	if !lf.takes(d.Kind) {
		return nil, fmt.Sprintf("kind %q is not one record.edge v%d lists (%s).", d.Kind, lf.def.Version, strings.Join(lf.kinds, ", "))
	}
	if why := d.SideProblem(); why != "" {
		return nil, why
	}
	if d.Confirm() && !d.Confirmed {
		return nil, fmt.Sprintf("This link names %d facets, more than %d. Check the list below, tick the confirmation and save again.",
			d.Count(), confirmAbove)
	}
	if why := pinProblem(d, stored); why != "" {
		return nil, why
	}
	if d.At == "" {
		return nil, "at: say when the link is asserted. The page fills it with now when your browser runs its script; type it when it does not."
	}
	fld := formField{Name: "at", Pointer: lf.at.timeAt, Input: "datetime-local", Value: d.At}
	at, err := lf.at.encodeTime(&fld)
	if err != nil {
		return nil, "at: " + err.Error()
	}
	st, ok := lf.at.sourceTime(vals, map[string]json.RawMessage{strings.TrimPrefix(lf.at.timeAt, "/"): at})
	d.Zone, d.Offset, d.OffsetProblem = lf.at.Zone, lf.at.Offset, lf.at.OffsetProblem
	if !ok {
		return nil, "The moment has no offset from UTC, so it names no instant."
	}
	var p edgePayload
	if err := json.Unmarshal(at, &p.At); err != nil {
		return nil, "at: " + err.Error() // encodeTime writes a string
	}
	p.SourceTime, p.Kind, p.Text = st, d.Kind, d.Text
	for _, it := range d.Items {
		if !it.Keep {
			continue
		}
		if it.From {
			p.From = append(p.From, it.ID)
		} else {
			p.To = append(p.To, it.ID)
		}
		if it.Pin {
			if p.Pins == nil {
				p.Pins = map[string]string{}
			}
			p.Pins[it.ID] = it.Hash
		}
	}
	p.From, p.To = sorted(p.From), sorted(p.To)
	p.ID = stored.ID
	if p.ID == "" {
		p.ID = edgeID(p.Kind, p.From, p.To, p.At, lf.at.Offset)
	}
	out, err := json.Marshal(p)
	if err != nil {
		return nil, err.Error() // a struct of strings always marshals
	}
	return out, ""
}

// pinProblem says why a pin cannot be written: the facet is not held, or the version
// the form showed is no longer current and is not the one the edited link pins
// already. A stale pin is moved to the current version for the person to check.
func pinProblem(d *linkData, stored edgeValue) string {
	var gone, moved []string
	for i := range d.Items {
		it := &d.Items[i]
		if !it.Keep || !it.Pin {
			continue
		}
		switch {
		case !it.Held:
			gone = append(gone, it.ID)
		case it.Hash != it.Current && it.Hash != stored.Pins[it.ID]:
			moved = append(moved, it.ID)
			it.Hash = it.Current
		}
	}
	switch {
	case len(gone) > 0:
		return fmt.Sprintf("Anam holds no facet %s, so there is no version to link to. Untick its \"link to this version\".",
			strings.Join(gone, ", "))
	case len(moved) > 0:
		return fmt.Sprintf("Facet %s has a newer version than the one this form showed. The form now links to the newest; "+
			"check it, then save again.", strings.Join(moved, ", "))
	}
	return ""
}
