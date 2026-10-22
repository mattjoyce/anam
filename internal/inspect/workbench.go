package inspect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/mattjoyce/anam/internal/facet"
)

// The Anam Workbench half of the inspector (card 188, S4): a form drawn from a
// Definition to enter a fact by hand, the same form on a facet the Workbench entered
// to correct it, and a button to retract one. The read pages gain no write branch:
// these three routes are their own handlers, they write only through the Workbench
// interface the daemon supplies, and a standalone inspector, given none, has no such
// routes at all.

// Workbench is what the entry pages need from the daemon, declared here where it is
// used. Source is the source the daemon enters facts as, and Enters says whether it
// may enter facts of a type, which the daemon asks the policy about the workbench
// principal in the importers file: the page asks and does not read that file. Enter submits one payload of a type, drawn at a
// Definition version, through the same write the envelope route ends in. Retract
// tombstones one facet of that source.
type Workbench interface {
	Source() string
	Enters(t facet.Type) bool
	Enter(ctx context.Context, t facet.Type, version int, payload json.RawMessage) (Entered, error)
	Retract(ctx context.Context, facetID string) (Retracted, error)
}

// Entered is what the gate answered an entry: admitted, superseded or no-op, the facet
// it holds, and the watermark.
type Entered struct {
	Outcome         string
	FacetID         string
	ArtifactVersion int
	W               int64
}

// Retracted is what the gate answered a retraction.
type Retracted struct {
	FacetID         string
	Outcome         string
	ArtifactVersion int
	W               int64
}

// DeclineKind says why a write was declined, so the page can answer with the right
// status and keep what was typed.
type DeclineKind int

// The reasons a write is declined: the gate refused the payload, the form was drawn
// from a Definition that is no longer in force, the facet is not one that can be
// retracted, the write queue is busy, or the facet is not held.
const (
	DeclinedRefused DeclineKind = iota + 1
	DeclinedStale
	DeclinedConflict
	DeclinedBusy
	DeclinedMissing
)

// Declined is an error the Workbench returns for a write that was refused, in the
// gate's own words. Nothing was written.
type Declined struct {
	Kind    DeclineKind
	Message string
}

func (e *Declined) Error() string { return e.Message }

// status is the HTTP status a declined write is answered with.
func (e *Declined) status() int {
	switch e.Kind {
	case DeclinedRefused:
		return http.StatusUnprocessableEntity
	case DeclinedStale, DeclinedConflict:
		return http.StatusConflict
	case DeclinedBusy:
		return http.StatusServiceUnavailable
	case DeclinedMissing:
		return http.StatusNotFound
	}
	return http.StatusInternalServerError
}

// maxFormBytes bounds a posted form. It is an invariant, not a default: the form is a
// handful of short fields, and the daemon bounds the payload it makes of them by
// max_payload_bytes again.
const maxFormBytes = 256 << 10

// enterable says whether the Workbench may enter facets of type t.
func (s *Server) enterable(t facet.Type) bool {
	return s.bench != nil && s.bench.Enters(t)
}

// entryData is the form page.
type entryData struct {
	Type          facet.Type
	Version       int
	Description   string
	Guidance      string
	IDRule        string
	DefinitionURL string
	Fields        []formField
	// Zoned says the form sends source_time from the browser's zone (card 198); Zone
	// and Offset are what it carries already: a correction's, or what was posted.
	Zoned         bool
	Zone, Offset  string
	OffsetProblem string
	// Edit is the facet being corrected, or "" for a new entry.
	Edit string
	// NotDrawn says why there is no form; Message is what the last save answered;
	// Notice is a line beside it.
	NotDrawn string
	Message  string
	Notice   string
	Locked   bool
}

// loadForm draws the form for type t from the Definition in force. It answers the
// request itself and returns false when it cannot: a type off the list is 403, a type
// with no Definition 404. A Definition with no form to draw comes back with NotDrawn
// set, for the caller to show.
func (s *Server) loadForm(ctx context.Context, w http.ResponseWriter, t facet.Type, lock bool) (entryData, entryForm, bool) {
	if !s.enterable(t) {
		http.Error(w, fmt.Sprintf("%s is not a type the Workbench enters by hand: only the types the importers file lists "+
			"for the workbench principal can be", t), http.StatusForbidden)
		return entryData{}, entryForm{}, false
	}
	reg, err := s.definitions()
	if err != nil {
		httpError(w, err)
		return entryData{}, entryForm{}, false
	}
	d, ok := reg.Lookup(t)
	if !ok {
		http.Error(w, fmt.Sprintf("no definition in force for %s", t), http.StatusNotFound)
		return entryData{}, entryForm{}, false
	}
	bodies, _, err := s.held(ctx)
	if err != nil {
		httpError(w, err)
		return entryData{}, entryForm{}, false
	}
	body, ok := bodies[t][d.Version]
	if !ok {
		http.Error(w, fmt.Sprintf("%s v%d is in force but its body is not held", t, d.Version), http.StatusInternalServerError)
		return entryData{}, entryForm{}, false
	}
	data := entryData{
		Type: t, Version: d.Version, Description: schemaDescription(body), IDRule: d.IdentityTemplate, Locked: lock,
		DefinitionURL: fmt.Sprintf("/definition?type=%s&v=%d", url.QueryEscape(string(t)), d.Version),
	}
	data.Guidance, _ = handlingOf(d)
	form, err := drawForm(body, d, lock)
	var notDrawn errNotDrawn
	if errors.As(err, &notDrawn) {
		data.NotDrawn = notDrawn.Error()
	}
	return data, form, true
}

// entryPage is GET /entry?type=<type>: the form, empty, or with ?edit=<facet id> the
// same form filled from that facet, its identity fields locked.
func (s *Server) entryPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	t := facet.Type(r.URL.Query().Get("type"))
	if t == "" {
		http.Error(w, "missing ?type=<facet type>", http.StatusBadRequest)
		return
	}
	edit := r.URL.Query().Get("edit")
	data, form, ok := s.loadForm(ctx, w, t, edit != "")
	if !ok {
		return
	}
	if data.NotDrawn == "" && edit != "" {
		if !s.fillFromFacet(ctx, w, &form, t, edit) {
			return
		}
		data.Edit = edit
	}
	data.Fields, data.Zoned, data.Zone, data.Offset = form.Fields, form.Zoned, form.Zone, form.Offset
	renderStatus(w, entryStatus(data), "entry.html", data)
}

// fillFromFacet loads the facet being corrected into the form.
func (s *Server) fillFromFacet(ctx context.Context, w http.ResponseWriter, form *entryForm, t facet.Type, id string) bool {
	f, ok := s.correctable(ctx, w, t, id)
	if !ok {
		return false
	}
	if err := form.fill(f.Payload.Bytes()); err != nil {
		httpError(w, err)
		return false
	}
	return true
}

// correctable is the current version of a facet the Workbench may correct: a live
// facet of type t that the Workbench itself entered. It answers the request itself and
// returns false for any other.
func (s *Server) correctable(ctx context.Context, w http.ResponseWriter, t facet.Type, id string) (facet.Facet, bool) {
	f, err := s.ledger.CurrentVersion(ctx, id)
	if err != nil {
		notFoundOr500(w, err)
		return facet.Facet{}, false
	}
	switch {
	case f.Type != t:
		http.Error(w, fmt.Sprintf("facet %s is a %s, not a %s", id, f.Type, t), http.StatusConflict)
		return facet.Facet{}, false
	case f.Provenance.Source != s.bench.Source():
		http.Error(w, fmt.Sprintf("facet %s came from %s: only a fact entered here, as %s, is corrected here",
			id, f.Provenance.Source, s.bench.Source()), http.StatusConflict)
		return facet.Facet{}, false
	}
	_, status, err := s.facetEvents(ctx, id)
	if err != nil {
		httpError(w, err)
		return facet.Facet{}, false
	}
	if status != facet.StatusAdmitted && status != facet.StatusEndorsed {
		http.Error(w, fmt.Sprintf("facet %s is %s, not live, so there is nothing to correct", id, status), http.StatusConflict)
		return facet.Facet{}, false
	}
	return f, true
}

// entryStatus is the status a form page is answered with: 422 for a type with no form.
func entryStatus(d entryData) int {
	if d.NotDrawn != "" {
		return http.StatusUnprocessableEntity
	}
	return http.StatusOK
}

// resultData is the page that answers a save or a retraction.
type resultData struct {
	Heading string
	Kind    string
	Lines   []string
	Links   []resultLink
}

type resultLink struct{ Href, Text string }

// entrySubmit is POST /entry: the form's fields are read back through the schema walk
// that drew them, and the typed payload goes to the Workbench. A refusal, a stale form
// or a busy queue draws the form again with what was typed and the gate's words.
func (s *Server) entrySubmit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t := facet.Type(r.PostForm.Get("type"))
	version, err := strconv.Atoi(r.PostForm.Get("v"))
	if err != nil || version < 1 {
		http.Error(w, "v: want the Definition version the form was drawn from", http.StatusBadRequest)
		return
	}
	edit := r.PostForm.Get("edit")
	data, form, ok := s.loadForm(ctx, w, t, edit != "")
	if !ok {
		return
	}
	data.Edit = edit
	if data.NotDrawn != "" {
		renderStatus(w, entryStatus(data), "entry.html", data)
		return
	}
	payload, typed := form.read(r.PostForm)
	data.Fields, data.Zoned, data.Zone, data.Offset = form.Fields, form.Zoned, form.Zone, form.Offset
	data.OffsetProblem = form.OffsetProblem
	if !typed {
		data.Message = "Some values are not what their fields take. Nothing was sent."
		renderStatus(w, http.StatusUnprocessableEntity, "entry.html", data)
		return
	}
	// The version the form was drawn from goes back with the payload, not the one in
	// force now, so a Definition that changed in between is refused by the gate.
	res, err := s.bench.Enter(ctx, t, version, payload)
	var declined *Declined
	switch {
	case errors.As(err, &declined):
		data.Message = declined.Message
		if declined.Kind == DeclinedStale {
			data.Notice = fmt.Sprintf("The form below is drawn from the Definition now in force, v%d. "+
				"Check it, then save again.", data.Version)
		}
		renderStatus(w, declined.status(), "entry.html", data)
	case err != nil:
		httpError(w, err)
	default:
		renderStatus(w, http.StatusOK, "result.html", enteredPage(t, edit, res))
	}
}

// enteredPage is the answer to a save, in the gate's own words.
func enteredPage(t facet.Type, edit string, res Entered) resultData {
	out := resultData{Heading: res.Outcome, Kind: res.Outcome}
	switch res.Outcome {
	case "admitted":
		out.Lines = []string{"A new facet was written."}
	case "superseded":
		out.Lines = []string{"A new version of a facet already held was written."}
	case "no-op":
		out.Lines = []string{"This facet, with this content, is already held, so nothing was written."}
	}
	out.Lines = append(out.Lines, fmt.Sprintf("Facet %s, artifact version %d, ledger watermark %d.",
		res.FacetID, res.ArtifactVersion, res.W))
	out.Links = []resultLink{{"/facet?id=" + url.QueryEscape(res.FacetID), "open the facet"}}
	if edit == "" {
		out.Links = append(out.Links, resultLink{"/entry?type=" + url.QueryEscape(string(t)), "enter another"})
	}
	return out
}

// retractSubmit is POST /retract: tombstone the facet the form names, which the
// Workbench refuses unless it is a live facet it entered.
func (s *Server) retractSubmit(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := r.PostForm.Get("id")
	if id == "" {
		http.Error(w, "id: want the facet to retract", http.StatusBadRequest)
		return
	}
	res, err := s.bench.Retract(ctx, id)
	var declined *Declined
	switch {
	case errors.As(err, &declined):
		renderStatus(w, declined.status(), "result.html", resultData{
			Heading: "not retracted", Kind: "rejected", Lines: []string{declined.Message, "Nothing was written."},
			Links: []resultLink{{"/facet?id=" + url.QueryEscape(id), "back to the facet"}},
		})
	case err != nil:
		httpError(w, err)
	default:
		heading := res.Outcome
		if heading == "tombstoned" {
			heading = "retracted"
		}
		renderStatus(w, http.StatusOK, "result.html", resultData{
			Heading: heading, Kind: res.Outcome,
			Lines: []string{fmt.Sprintf("Facet %s is tombstoned at artifact version %d, ledger watermark %d. "+
				"The record keeps the entry and the retraction.", res.FacetID, res.ArtifactVersion, res.W)},
			Links: []resultLink{{"/facet?id=" + url.QueryEscape(res.FacetID), "open the facet"}},
		})
	}
}

// renderStatus is render with a status other than 200.
func renderStatus(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := tmpl.ExecuteTemplate(&buf, name, data); err != nil {
		httpError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}

// workbenchActions says which Workbench buttons a facet page carries. Both need the
// newest version of a live facet that the Workbench entered; editing also needs its
// type to be on the list, since the form is refused otherwise. A facet of any other
// source gets neither.
func (s *Server) workbenchActions(f facet.Facet, latest int, status facet.Status) (edit, retract bool) {
	if s.bench == nil || f.Provenance.Source != s.bench.Source() || f.ArtifactVersion != latest {
		return false, false
	}
	if status != facet.StatusAdmitted && status != facet.StatusEndorsed {
		return false, false
	}
	return s.enterable(f.Type), true
}
