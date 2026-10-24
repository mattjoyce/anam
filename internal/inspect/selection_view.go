package inspect

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/read"
	"github.com/mattjoyce/anam/internal/selection"
)

// selectData is what the selection page draws.
type selectData struct {
	State    selectState
	Names    []string
	Selected string
	W        int64
	Served   int

	// Chips is false when the program is outside the chip grammar and runs as text.
	Chips bool
	// NotChips says why the program is outside the grammar.
	NotChips string
	Program  string
	// Problem is the evaluator's refusal of the program, in its words.
	Problem string
	// TooMany is the evaluator's refusal for size, or more rows than a page draws.
	TooMany bool

	Criteria []chip
	// Matched is how many facets the criteria match; Picked how many the program
	// selects, the ticks included and the exclusions not.
	Matched, Picked int
	RowCount        int
	Groups          []selectGroup
	Basket          []basketItem
	Excluded        []basketItem
	Handoff         string
	Limit           int
	// CanSave says the page can save its program under a code (card 206); Saved is
	// the saved selection the page was opened at, nil when it was not.
	CanSave bool
	Saved   *savedView
	// Types and Sources are what the served value holds, offered as the criteria's
	// suggestions.
	Types, Sources []string

	// labeled reads the label of a served facet; it is nil-safe and the zero value
	// labels nothing (card 199).
	labeled labels
}

// HasCriteria reports whether the selection says anything beyond its basket.
func (d selectData) HasCriteria() bool { return d.State.filter.HasCriteria() }

// chip is one criterion as drawn: its words, the address without it, and how many
// facets the criteria up to and including it match.
type chip struct {
	Label, Remove string
	Count         int
}

type selectGroup struct {
	Type string
	Rows []selectRow
}

// selectRow is one row of matches: the readings of one type and source at one fact
// time, folded together (one weigh-in's measures), each a member with its own box.
type selectRow struct {
	FactTime, Source string
	Members          []selectMember
}

type selectMember struct {
	ID      string
	Checked bool
	// Label is the facet's label from its Definition, "" when it has none.
	Label string
}

// Short is the id's first eight characters, enough to tell a row's members apart.
func (m selectMember) Short() string {
	if len(m.ID) > 8 {
		return m.ID[:8]
	}
	return m.ID
}

// basketItem is a ticked or excluded facet, described from the served value when it
// is there.
type basketItem struct {
	ID, Type, FactTime   string
	Label                string
	From                 bool
	Served               bool
	Remove, Side, Return string
}

func (s *Server) selectData(ctx context.Context, st selectState, served fold.Value, res fold.Result) (selectData, error) {
	templates, err := s.templates()
	if err != nil {
		return selectData{}, err
	}
	d := selectData{
		State: st, Names: append([]string{canonicalName}, templates.Names()...), Selected: st.proj,
		W: int64(res.W), Served: served.Len(), Chips: st.program == "", Program: st.program, Limit: rowLimit,
	}
	if d.Chips {
		if d.Program, err = st.filter.ProgramIn(st.zone.Name); err != nil {
			return selectData{}, err
		}
	} else if _, err := selection.Parse(st.program); err != nil {
		d.NotChips = err.Error()
	}
	d.Handoff = handoff(st, d.Program)

	defs, err := s.definitions()
	if err != nil {
		return selectData{}, err
	}
	ans, _, err := read.Evaluate(ctx, served, res.Live, defs, d.Program, selection.Relation, s.maxReadBytes)
	var tl *read.TooLarge
	switch {
	case errors.As(err, &tl):
		d.TooMany, d.Problem = true, tl.Error()
		return d, nil
	case err != nil && !d.Chips:
		d.Problem = err.Error()
		return d, nil
	case err != nil:
		return selectData{}, err
	}
	d.labeled = newLabels(defs, served)
	picked := map[string]bool{}
	for _, t := range ans.Tuples {
		picked[t[0].Text()] = true
	}
	d.Picked = len(picked)
	rows := rowsOf(served)
	d.Types, d.Sources = suggestions(rows)
	if d.Chips {
		d.fillChips(st, rows, picked)
	} else {
		d.fillText(rows, picked)
	}
	return d, nil
}

// suggestions is the types and sources among rows, each once and sorted.
func suggestions(rows []selection.Row) (types, sources []string) {
	t, s := map[string]bool{}, map[string]bool{}
	for _, r := range rows {
		t[string(r.Type)], s[r.Source] = true, true
	}
	return setOf(t), setOf(s)
}

func rowsOf(v fold.Value) []selection.Row {
	rows := make([]selection.Row, 0, v.Len())
	for _, e := range v.Entries() {
		rows = append(rows, selection.RowOf(e))
	}
	return rows
}

// fillChips draws the criteria with their counts, the matches with a box each
// (ticked while the program picks them), and the basket.
func (d *selectData) fillChips(st selectState, rows []selection.Row, picked map[string]bool) {
	d.Criteria = chips(st, rows)
	d.fillBasket(st, rows)
	if !st.filter.HasCriteria() {
		return
	}
	var matched []selection.Row
	for _, r := range rows {
		if st.filter.Matches(r) {
			matched = append(matched, r)
		}
	}
	d.Matched = len(matched)
	d.group(matched, picked)
}

// fillText lists what a program outside the grammar picks, without boxes: its ticks
// are in its text.
func (d *selectData) fillText(rows []selection.Row, picked map[string]bool) {
	var matched []selection.Row
	for _, r := range rows {
		if picked[r.ID] {
			matched = append(matched, r)
		}
	}
	d.Matched = len(matched)
	d.group(matched, picked)
}

// group folds the matches into rows by type, source and fact time, and draws none
// when there are more rows than a page holds: the answer is another criterion.
func (d *selectData) group(matched []selection.Row, picked map[string]bool) {
	type key struct{ typ, at, source string }
	at := map[key]*selectRow{}
	byType := map[string][]*selectRow{}
	for _, r := range matched {
		k := key{string(r.Type), r.FactTime, r.Source}
		row := at[k]
		if row == nil {
			row = &selectRow{FactTime: r.FactTime, Source: r.Source}
			at[k] = row
			byType[k.typ] = append(byType[k.typ], row)
		}
		row.Members = append(row.Members, selectMember{ID: r.ID, Checked: picked[r.ID], Label: d.labeled.of(r.ID)})
	}
	d.RowCount = len(at)
	if d.RowCount > rowLimit {
		d.TooMany = true
		return
	}
	for typ, rows := range byType {
		g := selectGroup{Type: typ}
		for _, r := range rows {
			sort.Slice(r.Members, func(i, j int) bool { return r.Members[i].ID < r.Members[j].ID })
			g.Rows = append(g.Rows, *r)
		}
		sort.Slice(g.Rows, func(i, j int) bool {
			a, b := g.Rows[i], g.Rows[j]
			if a.FactTime != b.FactTime {
				return a.FactTime < b.FactTime
			}
			return a.Source < b.Source
		})
		d.Groups = append(d.Groups, g)
	}
	sort.Slice(d.Groups, func(i, j int) bool { return d.Groups[i].Type < d.Groups[j].Type })
}

// chips is the criteria in the order the page draws them, each with the count the
// criteria so far match: the direct predicate, which a test holds equal to the
// program, so a count needs no evaluation of its own.
func chips(st selectState, rows []selection.Row) []chip {
	f := st.filter
	var out []chip
	var sofar selection.Filter
	add := func(label string, apply func(*selection.Filter), remove func(*selectState)) {
		apply(&sofar)
		n := 0
		for _, r := range rows {
			if sofar.Matches(r) {
				n++
			}
		}
		out = append(out, chip{Label: label, Count: n, Remove: st.with(remove).URL()})
	}
	if f.From != "" {
		add("from "+dayOf(f.From, false, st.zone.Loc), func(x *selection.Filter) { x.From = f.From }, func(c *selectState) { c.filter.From = "" })
	}
	if f.To != "" {
		add("to "+dayOf(f.To, true, st.zone.Loc), func(x *selection.Filter) { x.To = f.To }, func(c *selectState) { c.filter.To = "" })
	}
	// Types are a union, as are sources: each one's count is of the ones so far.
	for _, p := range f.Types {
		p := p
		add("type "+p, func(x *selection.Filter) { x.Types = append(x.Types, p) }, func(c *selectState) { c.filter.Types = without(c.filter.Types, p) })
	}
	for _, src := range f.Sources {
		src := src
		add("source "+src, func(x *selection.Filter) { x.Sources = append(x.Sources, src) }, func(c *selectState) { c.filter.Sources = without(c.filter.Sources, src) })
	}
	if f.Standing == selection.Superseded {
		add("standing superseded", func(x *selection.Filter) { x.Standing = selection.Superseded }, func(c *selectState) { c.filter.Standing = selection.Live })
	}
	return out
}

func without(xs []string, x string) []string {
	var out []string
	for _, y := range xs {
		if y != x {
			out = append(out, y)
		}
	}
	return out
}

// fillBasket describes each tick and exclusion, with the addresses that remove it,
// move it to the other side, or give an excluded facet back to the criteria.
func (d *selectData) fillBasket(st selectState, rows []selection.Row) {
	byID := make(map[string]selection.Row, len(rows))
	for _, r := range rows {
		byID[r.ID] = r
	}
	item := func(id string) basketItem {
		r, ok := byID[id]
		return basketItem{ID: id, Type: string(r.Type), FactTime: r.FactTime, Label: d.labeled.of(id), Served: ok, From: st.fromSide[id]}
	}
	for _, id := range st.filter.Ticks {
		id := id
		b := item(id)
		b.Remove = st.with(func(c *selectState) { c.filter.Ticks = without(c.filter.Ticks, id) }).URL()
		b.Side = st.with(func(c *selectState) { c.fromSide[id] = !c.fromSide[id] }).URL()
		d.Basket = append(d.Basket, b)
	}
	for _, id := range st.filter.Exclusions {
		id := id
		b := item(id)
		b.Return = st.with(func(c *selectState) { c.filter.Exclusions = without(c.filter.Exclusions, id) }).URL()
		d.Excluded = append(d.Excluded, b)
	}
}

// handoff is the address that hands the selection on to the link form (card 202): the
// program, the projection it was read through, the basket with each item's side, and
// the link being edited, if any. The form resolves the program to facet ids when it
// opens.
func handoff(st selectState, program string) string {
	q := url.Values{"program": {program}}
	if st.proj != canonicalName {
		q.Set("p", st.proj)
	}
	if st.edit != "" {
		q.Set("edit", st.edit)
	}
	q["tick"] = st.filter.Ticks
	st.addSides(q)
	return "/link?" + q.Encode()
}

// Hidden is the state as hidden inputs, for a form that changes one part of it.
func (d selectData) Hidden(except ...string) []hiddenInput {
	skip := index(except)
	var out []hiddenInput
	q := d.State.query()
	names := make([]string, 0, len(q))
	for k := range q {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if skip[k] {
			continue
		}
		for _, v := range q[k] {
			out = append(out, hiddenInput{Name: k, Value: v})
		}
	}
	return out
}

type hiddenInput struct{ Name, Value string }

// FromValue and ToValue are the bounds as the form's inputs show them.
func (d selectData) FromValue() string {
	return boundValue(d.State.filter.From, false, d.State.zone.Loc)
}

// ToValue is the to bound as the form shows it.
func (d selectData) ToValue() string { return boundValue(d.State.filter.To, true, d.State.zone.Loc) }

func boundValue(lexeme string, upper bool, loc *time.Location) string {
	if lexeme == "" {
		return ""
	}
	return dayOf(lexeme, upper, loc)
}

// Zone is the zone the page's dates are read in, for the field beside them.
func (d selectData) Zone() readZone { return d.State.zone }

// Superseded reports whether the filter reads versions no longer in force.
func (d selectData) Superseded() bool { return d.State.filter.Standing == selection.Superseded }

// CriteriaURL is the page at the same projection with no program and no criteria.
func (d selectData) CriteriaURL() string {
	return selectState{proj: d.State.proj, edit: d.State.edit}.URL()
}

// Editing is the link the selection will be handed back to, or "" for a new link.
func (d selectData) Editing() string { return d.State.edit }
