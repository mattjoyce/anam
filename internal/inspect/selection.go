package inspect

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/consent"
	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/selection"
	"github.com/mattjoyce/anam/internal/serve"
)

// The selection page (card 199): GET /select builds a Datalog selection. Criteria are
// added and removed in any order, the matches are listed grouped by type with a
// checkbox per facet, and the program the criteria and the ticks make is shown live
// and can be edited as text. The program is the value; the chips and checkboxes are a
// view of it, drawn only while it is within the chip grammar (selection.Parse).
//
// The page reads, and writes one thing: "save" files the program as an artifact under
// a short code (card 206, selection_saved.go), and ?sel=<code> opens it again, the
// chips drawn when it is within the grammar. Every state is an address: the criteria, the basket of ticked
// facets with each one's side, and the exclusions travel in the query string, so back
// and reload keep them. A form post of the checkboxes is answered with a redirect to
// the address of the new state. The selection is handed on by a link to card 202's
// link form, GET /link.

// selectState is one address of the page.
type selectState struct {
	proj   string
	filter selection.Filter
	// zone is the zone the filter's dates are read and written in (card 236).
	zone     readZone
	fromSide map[string]bool
	// program is a hand-edited program, "" while the criteria make it.
	program string
	// edit is the link being edited (card 202), carried through to the handoff so the
	// link form saves a new version of it rather than a new link; "" for a new one.
	edit string
}

var errBadSelect = errors.New("bad selection")

// stateOf reads the address. A date bound is a whole day in the zone z; a fact-time
// lexeme is an exact bound, which is how a hand-edited bound off midnight stays a chip.
func stateOf(q url.Values, z readZone) (selectState, error) {
	st := selectState{proj: q.Get("p"), program: q.Get("program"), edit: q.Get("edit"), zone: z, fromSide: map[string]bool{}}
	if st.proj == "" {
		st.proj = canonicalName
	}
	f := selection.Filter{
		Types: values(q["type"]), Sources: values(q["source"]), Standing: selection.Standing(q.Get("standing")),
		Ticks: values(q["tick"]), Exclusions: values(q["untick"]),
	}
	var err error
	if f.From, err = bound(q.Get("from"), false, z.Loc); err != nil {
		return st, err
	}
	if f.To, err = bound(q.Get("to"), true, z.Loc); err != nil {
		return st, err
	}
	if err := f.Validate(); err != nil {
		return st, fmt.Errorf("%w: %v", errBadSelect, err)
	}
	st.filter = f.Canonical()
	ticked := index(st.filter.Ticks)
	for _, id := range values(q["fromside"]) {
		if ticked[id] {
			st.fromSide[id] = true
		}
	}
	return st, nil
}

const canonicalName = "canonical"

// bound reads a from or to: a date in loc, a fact-time lexeme, or nothing. A to date is
// the whole day, so its bound is the next midnight.
func bound(s string, upper bool, loc *time.Location) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return s, nil // a lexeme, or Validate says why it is not
	}
	var lo, hi string
	var err error
	if upper {
		_, hi, err = selection.Days("", s, loc)
	} else {
		lo, _, err = selection.Days(s, "", loc)
	}
	if err != nil {
		return "", fmt.Errorf("%w: %v", errBadSelect, err)
	}
	return lo + hi, nil
}

// dayOf writes a bound back as the address and the form show it: a midnight in loc as
// its date (a to bound as the day before, since a to date is the whole day), anything
// else as the lexeme itself.
func dayOf(lexeme string, upper bool, loc *time.Location) string {
	t, err := clock.Parse(lexeme)
	if err != nil {
		return lexeme
	}
	if d, ok := dayIn(t, loc, upper); ok {
		return d
	}
	return lexeme
}

// values is xs without the empty values a form's blank inputs send.
func values(xs []string) []string {
	var out []string
	for _, x := range xs {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func index(xs []string) map[string]bool {
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}

// query is the state's address. The criteria and the basket are written in one
// order, so one state has one address however it was reached.
func (st selectState) query() url.Values {
	q := url.Values{}
	f := st.filter
	if st.proj != canonicalName {
		q.Set("p", st.proj)
	}
	if st.edit != "" {
		q.Set("edit", st.edit)
	}
	if st.zone.Name != "" && (st.zone.Given || f.From != "" || f.To != "") {
		q.Set("zone", st.zone.Name)
	}
	if st.program != "" {
		q.Set("program", st.program)
		st.addSides(q)
		return q
	}
	if f.From != "" {
		q.Set("from", dayOf(f.From, false, st.zone.Loc))
	}
	if f.To != "" {
		q.Set("to", dayOf(f.To, true, st.zone.Loc))
	}
	q["type"], q["source"] = f.Types, f.Sources
	if f.Standing == selection.Superseded {
		q.Set("standing", string(f.Standing))
	}
	q["tick"], q["untick"] = f.Ticks, f.Exclusions
	st.addSides(q)
	for k, v := range q {
		if len(v) == 0 {
			delete(q, k)
		}
	}
	return q
}

func (st selectState) addSides(q url.Values) {
	for _, id := range st.filter.Ticks {
		if st.fromSide[id] {
			q.Add("fromside", id)
		}
	}
}

// URL is the state's address on this page.
func (st selectState) URL() string { return address("/select", st.query()) }

// with is a copy of the state changed by edit, the fromSide set copied too.
func (st selectState) with(edit func(*selectState)) selectState {
	c := st
	c.fromSide = make(map[string]bool, len(st.fromSide))
	for k, v := range st.fromSide {
		c.fromSide[k] = v
	}
	c.filter.Types = append([]string(nil), st.filter.Types...)
	c.filter.Sources = append([]string(nil), st.filter.Sources...)
	c.filter.Ticks = append([]string(nil), st.filter.Ticks...)
	c.filter.Exclusions = append([]string(nil), st.filter.Exclusions...)
	edit(&c)
	c.filter = c.filter.Canonical()
	return c
}

func (s *Server) selectPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	q := r.URL.Query()
	z, ok := s.zoneOrFail(w, q)
	if !ok {
		return
	}
	st, err := stateOf(q, z)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if name := selection.ZoneOf(st.program); name != "" {
		// A program says the zone its dates were read in; the page reads it so.
		if st.zone, err = zoneNamed(name); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		st.zone.Given, st.zone.Dated = true, true
	}
	if _, ok := q["shown"]; ok {
		http.Redirect(w, r, address("/select", applyTicks(st, q).query()), http.StatusSeeOther)
		return
	}
	var saved *selection.Saved
	if code := q.Get("sel"); code != "" {
		sv, ok := s.openSelection(r.Context(), w, code)
		if !ok {
			return
		}
		saved = &sv
		if st.zone, err = savedZone(sv); err != nil {
			httpError(w, err)
			return
		}
		st.filter, st.program = selection.Filter{Standing: selection.Live}, sv.Program
		if f, err := selection.Parse(sv.Program); err == nil {
			st.filter, st.program = f, "" // within the grammar: the chips are drawn from it
		}
	}
	if st.program != "" && saved == nil {
		if f, err := selection.Parse(st.program); err == nil {
			// Within the grammar: the chips are drawn from it.
			next := st.with(func(c *selectState) { c.filter, c.program = f, "" })
			http.Redirect(w, r, address("/select", next.query()), http.StatusSeeOther)
			return
		}
	}
	served, res, err := s.servedFor(ctx, st.proj)
	switch {
	case errors.Is(err, errUnknownProjection):
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	case err != nil:
		httpError(w, err)
		return
	}
	data, err := s.selectData(ctx, st, served, res)
	if err != nil {
		httpError(w, err)
		return
	}
	data.CanSave = s.selections != nil
	data.Timeline = s.timelineFor(st, served, saved)
	if saved != nil {
		v := savedViewOf(*saved)
		if selection.ZoneOf(saved.Program) != "" || st.filter.From != "" || st.filter.To != "" {
			v.Zone = st.zone.Name
		}
		data.Saved = &v
	}
	s.render(w, "select.html", data)
}

// applyTicks reads the checkboxes posted back: a listed facet left unticked is an
// exclusion, and dropped from the ticks; a ticked one is not excluded. "keep" also
// puts every ticked one in the basket, so it outlives the criteria that found it.
func applyTicks(st selectState, q url.Values) selectState {
	on := index(q["on"])
	keep := q.Get("act") == "keep"
	return st.with(func(c *selectState) {
		ex, ticks := index(c.filter.Exclusions), index(c.filter.Ticks)
		for _, id := range values(q["shown"]) {
			switch {
			case !on[id]:
				ex[id], ticks[id] = true, false
				delete(c.fromSide, id)
			case keep:
				ex[id], ticks[id] = false, true
			default:
				ex[id] = false
			}
		}
		c.filter.Exclusions, c.filter.Ticks = setOf(ex), setOf(ticks)
	})
}

// setOf is the members of m marked true, sorted.
func setOf(m map[string]bool) []string {
	var out []string
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

var errUnknownProjection = errors.New("unknown projection")

// servedFor is the value a projection serves at the head, the way the query read
// derives it (serve.Derive), or the canonical itself, which is the un-narrowed read.
func (s *Server) servedFor(ctx context.Context, name string) (fold.Value, fold.Result, error) {
	res, err := s.read(ctx)
	if err != nil {
		return fold.Value{}, fold.Result{}, err
	}
	if name == canonicalName {
		return res.Value, res, nil
	}
	templates, err := s.templates()
	if err != nil {
		return fold.Value{}, fold.Result{}, err
	}
	t, err := templates.Lookup(name)
	if err != nil {
		return fold.Value{}, fold.Result{}, fmt.Errorf("%w: %v", errUnknownProjection, err)
	}
	defs, err := s.definitions()
	if err != nil {
		return fold.Value{}, fold.Result{}, err
	}
	served, err := serve.Derive(res.Value, t, consent.Open{}, defs, "")
	return served, res, err
}

// timelineFor is the "show on timeline" address for the state: its filter's window and
// types, the zone, and the saved selection when the page was opened at one. A program
// outside the criteria has no window to show, so only a saved one links, by its code.
func (s *Server) timelineFor(st selectState, served fold.Value, saved *selection.Saved) string {
	code := ""
	if saved != nil {
		code = saved.Code
	}
	if st.program != "" {
		if code == "" {
			return ""
		}
		return address("/", url.Values{"sel": {code}})
	}
	types, _ := suggestions(rowsOf(served))
	return st.timelineLink(types, code)
}
