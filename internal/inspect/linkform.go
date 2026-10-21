package inspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/catchup"
	"github.com/mattjoyce/anam/internal/def"
)

// The link form (card 202): GET /link draws one record.edge from a basket of facets,
// each on its side, with a kind, the person's words and a pin per facet; POST /link
// saves it through the Workbench, as the entry form saves a dose. The basket comes from
// the selection page's handoff (its program, resolved to facet ids when the form
// opens, and its ticks and sides), from a facet page's "edit this link", or is empty.
// The entry form cannot draw record.edge, whose sides are lists, so this is its form;
// the at field, its zone and its offset are read and written by the entry form's code.

// confirmAbove is how many facets a link may name before the form asks the person to
// confirm it (Matt, 2026-10-21): the selection page starts its matches ticked, so one
// broad criterion can fill a basket unseen. It is presentation, like a page size, and
// not a default: it limits nothing, since a confirmed link of any size is saved.
const confirmAbove = 20

// edgeIDPrefix starts the id of every link the Workbench writes.
const edgeIDPrefix = "workbench:edge:"

// edgeID is the id of a link the Workbench writes, computed once at its first save and
// kept for every later version (Matt, 2026-10-21), so an edit that changes the sides is
// a new version of the same link and the same link saved twice is one fact. The rule:
//
//	"workbench:edge:" + the first 16 hex digits of the SHA-256 of
//	"kind=" + kind + "\nfrom=" + from + "\nto=" + to + "\nat=" + at + offset
//
// where from and to are each side's facet ids sorted and joined by ",", at is the
// payload's at as written ("2026-10-21 14:30"), and offset is source_time's offset
// ("+11:00"), so "at=2026-10-21 14:30+11:00" names one instant. A facet id holds no
// comma and no newline, so the string has one reading. The text is not in it: the
// same facets joined at the same moment with other words are a correction of the
// words. Record.edge declares no identity_template for this, since the gate would
// compose the id from the edited sides again and turn an edit into a second link.
func edgeID(kind string, from, to []string, at, offset string) string {
	canonical := "kind=" + kind + "\nfrom=" + strings.Join(sorted(from), ",") +
		"\nto=" + strings.Join(sorted(to), ",") + "\nat=" + at + offset
	sum := sha256.Sum256([]byte(canonical))
	return edgeIDPrefix + hex.EncodeToString(sum[:])[:16]
}

// sorted is a sorted copy of xs.
func sorted(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

// noFromSide is the kinds that take nothing on the from side. Record.edge's Definition
// says so in its kind's description (about: "the person's words about the facets in
// `to`, no `from`") and declares no rule a page can read, so the form holds this one;
// card 199's per-kind side rules would move it into the Definition.
var noFromSide = map[string]bool{"about": true}

// unpinnedByDefault is the kinds whose facets the form leaves unpinned until the person
// ticks "link to this version" (card 199): a note about a facet follows it through its
// versions. Every other kind starts pinned, as interpreted_from is a reading of exact
// words.
var unpinnedByDefault = map[string]bool{"about": true}

// linkItem is one facet of the basket as the form draws it.
type linkItem struct {
	ID string
	// Type and FactTime describe the facet's current version; Held is false for an id
	// the ledger does not hold.
	Type, FactTime string
	Held           bool
	From, Keep     bool
	Pin            bool
	// Hash is the version a pin names: the one an edited link pins already, or the one
	// current when the form was drawn. Current is the current version's.
	Hash, Current string
}

// ShortHash is the pinned hash's first digits, enough to tell versions apart on a page.
func (it linkItem) ShortHash() string {
	h := strings.TrimPrefix(it.Hash, "sha256:")
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// linkData is the link form's page.
type linkData struct {
	Version       int
	DefinitionURL string
	Kinds         []string
	Kind          string
	Text          string
	Items         []linkItem
	// At is the moment as the datetime-local control holds it; Zone and Offset are
	// what the page sends as source_time, filled from the browser as the dose form does.
	At, Zone, Offset string
	OffsetProblem    string
	// Edit is the link being edited, "" for a new one.
	Edit string
	// Message is why the last save wrote nothing; Notice a line beside it; Problem why
	// the basket could not be read from the selection handed on.
	Message, Notice, Problem string
	Confirmed                bool
	ConfirmAbove             int
}

// Count is how many facets the link names: the basket's kept items.
func (d linkData) Count() int {
	n := 0
	for _, it := range d.Items {
		if it.Keep {
			n++
		}
	}
	return n
}

// Confirm says the form asks for confirmation before saving.
func (d linkData) Confirm() bool { return d.Count() > confirmAbove }

// SideProblem is the message for a basket the kind's side rules refuse, or "".
func (d linkData) SideProblem() string {
	if !noFromSide[d.Kind] {
		return ""
	}
	from := d.fromSide()
	if len(from) == 0 {
		return ""
	}
	return fmt.Sprintf("A link of kind %s has no from side: it is your words about the facets on the to side. "+
		"Move %s to the to side, or choose a kind that takes a from side.", d.Kind, strings.Join(from, ", "))
}

// fromSide is the ids of the kept items on the from side, its tests plain ifs as
// pinned's are.
func (d linkData) fromSide() []string {
	var from []string
	for _, it := range d.Items {
		if !it.Keep {
			continue
		}
		if it.From {
			from = append(from, it.ID)
		}
	}
	return from
}

// SelectURL opens the selection page with this basket, each facet on its side, to add
// or remove facets; it comes back here through the selection's handoff.
func (d linkData) SelectURL() string {
	q := url.Values{}
	for _, it := range d.Items {
		if !it.Keep {
			continue
		}
		q.Add("tick", it.ID)
		if it.From {
			q.Add("fromside", it.ID)
		}
	}
	if d.Edit != "" {
		q.Set("edit", d.Edit)
	}
	return "/select?" + q.Encode()
}

// linkForm is record.edge's Definition in force, read for the form: its kinds, and the
// at field's layout and zone as the entry form reads a fact time.
type linkForm struct {
	def   def.Definition
	kinds []string
	at    entryForm
}

// loadLinkForm reads record.edge's Definition in force. It answers the request itself
// and returns false when the Workbench may not enter links or the Definition cannot be
// read.
func (s *Server) loadLinkForm(ctx context.Context, w http.ResponseWriter) (linkForm, bool) {
	t := catchup.EdgeType
	if !s.enterable(t) {
		http.Error(w, fmt.Sprintf("%s is not a type the Workbench enters by hand: only the types the importers file lists "+
			"for the workbench principal can be, so add it there to write links here", t), http.StatusForbidden)
		return linkForm{}, false
	}
	reg, err := s.definitions()
	if err != nil {
		httpError(w, err)
		return linkForm{}, false
	}
	d, ok := reg.Lookup(t)
	if !ok {
		http.Error(w, fmt.Sprintf("no definition in force for %s", t), http.StatusNotFound)
		return linkForm{}, false
	}
	bodies, _, err := s.held(ctx)
	if err != nil {
		httpError(w, err)
		return linkForm{}, false
	}
	kinds, err := kindsOf(bodies[t][d.Version])
	if err != nil {
		http.Error(w, fmt.Sprintf("%s v%d: %v", t, d.Version, err), http.StatusInternalServerError)
		return linkForm{}, false
	}
	loc, err := time.LoadLocation(d.FactTime.Zone)
	if err != nil {
		http.Error(w, fmt.Sprintf("%s v%d: the fact-time zone %q cannot be loaded: %v", t, d.Version, d.FactTime.Zone, err),
			http.StatusInternalServerError)
		return linkForm{}, false
	}
	at := entryForm{timeAt: d.FactTime.Pointer, layout: d.FactTime.Layout, loc: loc, Zoned: d.FactTime.ZonePointer != ""}
	return linkForm{def: d, kinds: kinds, at: at}, true
}

// kindsOf is the kind field's closed list in a record.edge body.
func kindsOf(body []byte) ([]string, error) {
	var m struct {
		Schema struct {
			Properties struct {
				Kind struct {
					Enum []string `json:"enum"`
				} `json:"kind"`
			} `json:"properties"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("the body is not a Definition: %w", err)
	}
	if len(m.Schema.Properties.Kind.Enum) == 0 {
		return nil, errors.New("the Definition lists no kinds")
	}
	return m.Schema.Properties.Kind.Enum, nil
}

// page is an empty form at the Definition in force.
func (lf linkForm) page() linkData {
	d := linkData{
		Version: lf.def.Version, Kinds: lf.kinds, Kind: lf.kinds[0], ConfirmAbove: confirmAbove,
		DefinitionURL: fmt.Sprintf("/definition?type=%s&v=%d", url.QueryEscape(string(catchup.EdgeType)), lf.def.Version),
	}
	if lf.takes("about") {
		d.Kind = "about" // absent means about, so it is the kind a link has unless said
	}
	return d
}

// takes says the Definition lists kind.
func (lf linkForm) takes(kind string) bool {
	for _, k := range lf.kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// linkPage is GET /link: the form, with the basket its query string names (program or
// sel, p, tick, fromside, and kind), or with ?edit=<facet id> the link being edited, its sides
// replaced by the basket when the selection page hands one back.
func (s *Server) linkPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	lf, ok := s.loadLinkForm(ctx, w)
	if !ok {
		return
	}
	q := r.URL.Query()
	data := lf.page()
	var pins map[string]string
	if edit := q.Get("edit"); edit != "" {
		stored, ok := s.editableEdge(ctx, w, edit)
		if !ok {
			return
		}
		lf.fillFrom(&data, stored)
		data.Edit, pins = edit, stored.Pins
	} else if k := q.Get("kind"); lf.takes(k) {
		data.Kind = k
	}
	if code := q.Get("sel"); code != "" {
		// A saved selection (card 206) fills the basket as its program does.
		saved, ok := s.openSelection(w, code)
		if !ok {
			return
		}
		q.Set("program", saved.Program)
	}
	if q.Has("tick") || q.Has("program") {
		ids, problem, err := s.basketOf(ctx, q)
		if err != nil {
			httpError(w, err)
			return
		}
		data.Problem = problem
		data.Items = itemsOf(ids, index(values(q["fromside"])), pins, data.Kind)
	}
	if err := s.describe(ctx, &data); err != nil {
		httpError(w, err)
		return
	}
	render(w, "link.html", data)
}
