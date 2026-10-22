package inspect

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/selection"
)

// Saved selections (card 206): the selection page saves its program under a short
// code, sel:<code>, and the selection page, the timeline and the link form open one
// by ?sel=<code>. Saving writes an artifact and nothing else, never the ledger.

// Selections saves a program under its code and opens the program a code names;
// *selection.Store satisfies it.
type Selections interface {
	Save(ctx context.Context, text string) (selection.Saved, error)
	Open(ctx context.Context, code string) (selection.Saved, error)
}

// missingData is the page for a code that opens nothing.
type missingData struct {
	Code, Message string
}

// openSelection is the saved selection the code names. It answers the request
// itself and returns false when there is none: 404 for a code nothing was saved
// under, 400 for one that cannot be a code, 409 for one that names two. An unknown
// code is never drawn as an empty page, which would read as a selection of nothing.
func (s *Server) openSelection(ctx context.Context, w http.ResponseWriter, code string) (selection.Saved, bool) {
	shown := strings.TrimPrefix(strings.TrimSpace(code), selection.CodePrefix)
	if s.selections == nil {
		renderStatus(w, http.StatusNotFound, "selection_missing.html", missingData{Code: shown,
			Message: "This inspector keeps no saved selections: they are the daemon's. Open the daemon's inspector, or run the program itself."})
		return selection.Saved{}, false
	}
	saved, err := s.selections.Open(ctx, code)
	if err == nil {
		return saved, true
	}
	var status int
	switch {
	case errors.Is(err, selection.ErrNoCode):
		status = http.StatusNotFound
	case errors.Is(err, selection.ErrBadCode):
		status = http.StatusBadRequest
	case errors.Is(err, selection.ErrAmbiguous):
		status = http.StatusConflict
	default:
		httpError(w, err)
		return selection.Saved{}, false
	}
	renderStatus(w, status, "selection_missing.html", missingData{Code: shown, Message: err.Error()})
	return selection.Saved{}, false
}

// saveSelection is POST /select/save: the program in the form is saved, and the
// browser lands on the selection page at its code, which shows the code to copy.
func (s *Server) saveSelection(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	saved, err := s.selections.Save(r.Context(), r.PostForm.Get("program"))
	switch {
	case errors.Is(err, selection.ErrNotProgram):
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	case err != nil:
		httpError(w, err)
		return
	}
	q := url.Values{"sel": {saved.Code}}
	if p := r.PostForm.Get("p"); p != "" && p != canonicalName {
		q.Set("p", p)
	}
	if e := r.PostForm.Get("edit"); e != "" {
		q.Set("edit", e)
	}
	http.Redirect(w, r, "/select?"+q.Encode(), http.StatusSeeOther)
}

// savedView is a saved selection as the selection page shows it: its code and the
// addresses that open it.
type savedView struct {
	Code, Ref, Hash                 string
	SelectURL, TimelineURL, LinkURL string
	HighlightURL                    string
}

func savedViewOf(saved selection.Saved) savedView {
	c := url.QueryEscape(saved.Code)
	return savedView{
		Code: saved.Code, Ref: saved.Ref(), Hash: saved.Hash,
		SelectURL: "/select?sel=" + c, TimelineURL: "/?sel=" + c, LinkURL: "/link?sel=" + c,
		HighlightURL: "/?sel=" + c + "&selmode=" + selHighlight,
	}
}

// The timeline's two ways of drawing a selection (selmode): only its facets, or
// every facet with its own picked out.
const (
	selOnly      = "only"
	selHighlight = "highlight"
)

func parseSelMode(q string) (string, error) {
	switch q {
	case "", selOnly:
		return selOnly, nil
	case selHighlight:
		return selHighlight, nil
	}
	return "", fmt.Errorf("selmode: %q is not %s or %s", q, selOnly, selHighlight)
}

// tlSelection is a saved selection drawn on the timeline.
type tlSelection struct {
	Code, Mode string
	// Picked is the facets the program selects among those served in the window.
	Picked map[string]bool
	// Count is how many of them the band draws.
	Count int
	// TooLarge says the program grew past max_read_bytes over this window; Problem
	// is the evaluator's refusal of the program otherwise. Either draws no picks.
	TooLarge, Problem string
}

// Highlight reports whether the band draws every facet with the selection's own
// picked out.
func (t *tlSelection) Highlight() bool { return t != nil && t.Mode == selHighlight }

// pickedIn runs a saved program over the canonical's facets whose fact time lies in
// the window, through read.Evaluate as the selection page runs it, bounded by
// max_read_bytes. The window bounds the program's input, so a selection too large
// for the whole record can be drawn a narrower window at a time.
func (s *Server) pickedIn(ctx context.Context, res fold.Result, saved selection.Saved, from, to time.Time) (*tlSelection, error) {
	sel := &tlSelection{Code: saved.Code, Picked: map[string]bool{}}
	lo, hi := clock.Format(from), clock.Format(to)
	var in []fold.Entry
	for _, e := range res.Value.Entries() {
		if e.FactTime < lo || e.FactTime > hi {
			continue
		}
		in = append(in, e)
	}
	window, err := fold.NewValue(in)
	if err != nil {
		return nil, err
	}
	defs, err := s.definitions()
	if err != nil {
		return nil, err
	}
	ans, _, err := read.Evaluate(ctx, window, res.Live, defs, saved.Program, selection.Relation, s.maxReadBytes)
	var tl *read.TooLarge
	switch {
	case errors.As(err, &tl):
		sel.TooLarge = fmt.Sprintf("over this window it grows past %d bytes (max_read_bytes)", tl.MaxBytes)
		return sel, nil
	case errors.Is(err, read.ErrQuery):
		sel.Problem = err.Error()
		return sel, nil
	case err != nil:
		return nil, err
	}
	for _, t := range ans.Tuples {
		sel.Picked[t[0].Text()] = true
	}
	return sel, nil
}

// ref is the selection as the timeline's links carry it; nil carries none.
func (t *tlSelection) ref() selRef {
	if t == nil {
		return selRef{}
	}
	return selRef{Code: t.Code, Mode: t.Mode}
}

// drawSelection opens the saved selection ?sel= names for the timeline and runs it
// over the window. Drawn only (the default), the band holds its facets alone, and a
// window the person did not choose fits them; highlighted, it holds every facet. It
// answers the request itself and returns false for a code that opens nothing.
func (s *Server) drawSelection(ctx context.Context, w http.ResponseWriter, q url.Values, res fold.Result,
	all []facet.Facet, from, to time.Time, zoomed bool,
) (*tlSelection, []facet.Facet, time.Time, time.Time, bool) {
	mode, err := parseSelMode(q.Get("selmode"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return nil, nil, from, to, false
	}
	saved, ok := s.openSelection(ctx, w, q.Get("sel"))
	if !ok {
		return nil, nil, from, to, false
	}
	sel, err := s.pickedIn(ctx, res, saved, from, to)
	if err != nil {
		httpError(w, err)
		return nil, nil, from, to, false
	}
	sel.Mode = mode
	if mode == selHighlight {
		return sel, all, from, to, true
	}
	var only []facet.Facet
	for _, f := range all {
		if sel.Picked[f.ID] {
			only = append(only, f)
		}
	}
	if !zoomed && len(only) > 0 {
		// The window was the whole record's, so the program ran over all of it;
		// fitting it to what was picked changes what is drawn, not what is picked.
		from, to, _, _ = parseWindow("", "", only)
	}
	return sel, only, from, to, true
}

// selNav is the selection's links over the window: drawn only, highlighted, and the
// view without it; and the selection page at its code.
func selNav(from, to time.Time, zoomed bool, view tlView) tlSelNav {
	code := view.Sel.Code
	at := func(mode string) string {
		v := view
		v.Sel = selRef{Code: code, Mode: mode}
		return windowLink(from, to, zoomed, v)
	}
	without := view
	without.Sel = selRef{}
	return tlSelNav{
		Only: at(selOnly), Highlight: at(selHighlight), Clear: windowLink(from, to, zoomed, without),
		Select: "/select?sel=" + url.QueryEscape(code),
	}
}
