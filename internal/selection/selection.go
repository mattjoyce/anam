// Package selection is the shared filter (card 199): the criteria a person adds in
// any order to find facets, fact-time from and to, a type or a type prefix, a source,
// a standing, and the facets ticked or unticked by hand. One type, used by the
// selection page, the projection page's rows view (card 201) and migrate (card 200).
//
// The filter is a value with two readings that a test holds equal. Matches and Picks
// are its direct predicate, plain Go over a row. Program compiles it to Datalog, a
// unary relation pick/1 over the query surface's entry/7 (internal/query), and the
// program text is the value a selection is handed on as: shown, copied, edited, run
// through POST /v1/query. Parse reads a program back to a filter only when it is one
// Program could have written, and says so when it is not, so a hand-edited program
// beyond the chips (a join, a recursion, an attr) still runs, as text.
//
// Everything here is read-side and disposable: it reads rows and writes nothing.
package selection

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// Standing says which versions a filter reads: those in force, or those no longer.
type Standing string

// The standings. Live is the default: a facet's version in force, admitted or
// endorsed, which is everything a served value holds. Superseded is a version no
// longer in force: displaced by a later one, or the current version of a facet that
// was retracted or rejected. A served value holds none, so over one a superseded
// filter picks nothing; the projection page's superseded rows (card 201) are where
// it selects.
const (
	Live       Standing = "live"
	Superseded Standing = "superseded"
)

// ErrInvalid is returned for a filter whose criteria cannot be written as a program:
// a type pattern that is not one, a bound that is not a fact-time, an empty id.
var ErrInvalid = errors.New("selection: invalid filter")

// Filter is the criteria and the hand-picks. It is a set: the order of the values in
// each list and their repeats are not part of it (Canonical).
type Filter struct {
	// From and To bound fact-time, as fact-time lexemes (clock.Layout): From is
	// included, To is not, and "" is open. Days returns the bounds of whole days.
	From, To string
	// Types are type patterns (facet.ValidPattern): an exact type, or a prefix ending
	// in "." that matches at a segment boundary (card 193). Any one admits a row.
	Types []string
	// Sources are source names; any one admits a row.
	Sources []string
	// Standing is Live when empty.
	Standing Standing
	// Ticks are facet ids picked by hand, in the selection whatever the criteria say.
	Ticks []string
	// Exclusions are facet ids the criteria admit and the person unticked.
	Exclusions []string
}

// Row is what the direct predicate reads of one facet version.
type Row struct {
	ID       string
	Type     facet.Type
	FactTime string
	Source   string
	// Live is whether the version is the one in force, admitted or endorsed.
	Live bool
}

// RowOf is the row of one served entry.
func RowOf(e fold.Entry) Row {
	return Row{
		ID: e.FacetID, Type: e.Type, FactTime: e.FactTime, Source: e.Source,
		Live: fold.State{Status: e.Status}.Live(),
	}
}

// Canonical is the filter as a set: each list sorted with its repeats removed, an
// exclusion that is also a tick dropped (the tick wins), the standing named. Empty
// lists are nil, so two canonical filters compare equal exactly when they select by
// the same criteria and picks.
func (f Filter) Canonical() Filter {
	out := Filter{
		From: f.From, To: f.To, Standing: f.Standing,
		Types: set(f.Types), Sources: set(f.Sources), Ticks: set(f.Ticks),
	}
	if out.Standing == "" {
		out.Standing = Live
	}
	ticked := index(out.Ticks)
	var ex []string
	for _, id := range set(f.Exclusions) {
		if !ticked[id] {
			ex = append(ex, id)
		}
	}
	out.Exclusions = ex
	return out
}

// HasCriteria reports whether the filter says anything beyond its hand-picks. A
// filter without criteria selects its ticks and nothing else, not the whole record.
func (f Filter) HasCriteria() bool {
	return f.From != "" || f.To != "" || len(f.Types) > 0 || len(f.Sources) > 0 || f.Standing == Superseded
}

// Validate refuses a filter Program cannot write, saying which value and why.
func (f Filter) Validate() error {
	for _, b := range []struct{ name, v string }{{"from", f.From}, {"to", f.To}} {
		if b.v != "" && !isLexeme(b.v) {
			return fmt.Errorf("%w: %s %q is not a fact-time; write the whole instant (layout %s)", ErrInvalid, b.name, b.v, clock.Layout)
		}
	}
	for _, p := range f.Types {
		if !facet.ValidPattern(p) {
			return fmt.Errorf("%w: %q is not a type or a type prefix (a prefix ends in \".\" and names at least a domain and a concept)", ErrInvalid, p)
		}
	}
	for _, s := range f.Sources {
		if s == "" {
			return fmt.Errorf("%w: an empty source", ErrInvalid)
		}
	}
	for _, id := range append(append([]string(nil), f.Ticks...), f.Exclusions...) {
		if id == "" {
			return fmt.Errorf("%w: an empty facet id", ErrInvalid)
		}
	}
	switch f.Standing {
	case "", Live, Superseded:
		return nil
	}
	return fmt.Errorf("%w: standing %q is not %s or %s", ErrInvalid, f.Standing, Live, Superseded)
}

// Matches reports whether every criterion admits r. Ticks and exclusions play no
// part: it is the question the criteria ask. A filter without criteria matches every
// row, which is the count before the first criterion.
func (f Filter) Matches(r Row) bool {
	return f.inWindow(r.FactTime) && f.typeMatches(r.Type) && f.sourceMatches(r.Source) && f.standingMatches(r.Live)
}

// MatchesType reports whether the filter's types admit t: true when it names none.
func (f Filter) MatchesType(t facet.Type) bool { return f.typeMatches(t) }

func (f Filter) inWindow(at string) bool {
	return (f.From == "" || at >= f.From) && (f.To == "" || at < f.To)
}

func (f Filter) typeMatches(t facet.Type) bool {
	if len(f.Types) == 0 {
		return true
	}
	for _, p := range f.Types {
		if facet.Matches(t, p) {
			return true
		}
	}
	return false
}

func (f Filter) sourceMatches(s string) bool {
	if len(f.Sources) == 0 {
		return true
	}
	for _, x := range f.Sources {
		if x == s {
			return true
		}
	}
	return false
}

func (f Filter) standingMatches(live bool) bool {
	if f.Standing == Superseded {
		return !live
	}
	return live
}

// Picks is the direct predicate's answer over rows: the ids of the rows the criteria
// match and the person did not untick, when there are criteria, and every tick
// whether or not it is among the rows. Sorted, each once. It is what the program
// answers as pick/1 over the same rows.
func (f Filter) Picks(rows []Row) []string {
	out := append([]string(nil), f.Ticks...)
	if f.HasCriteria() {
		excluded := index(f.Exclusions)
		for _, r := range rows {
			if f.Matches(r) && !excluded[r.ID] {
				out = append(out, r.ID)
			}
		}
	}
	return set(out)
}

// Days is the fact-time bounds of whole days: from the start of from to the start of
// the day after to, so a range of one day is from and to the same date. Either may be
// "" for open. The days are UTC's, as the projection page's are.
func Days(from, to string) (string, string, error) {
	var lo, hi string
	if from != "" {
		t, err := time.Parse(time.DateOnly, from)
		if err != nil {
			return "", "", fmt.Errorf("%w: from %q is not YYYY-MM-DD", ErrInvalid, from)
		}
		lo = clock.Format(t)
	}
	if to != "" {
		t, err := time.Parse(time.DateOnly, to)
		if err != nil {
			return "", "", fmt.Errorf("%w: to %q is not YYYY-MM-DD", ErrInvalid, to)
		}
		hi = clock.Format(t.AddDate(0, 0, 1))
	}
	return lo, hi, nil
}

// isLexeme reports whether s is a fact-time exactly as the record writes one, so the
// lexical comparison the program makes is the chronological one.
func isLexeme(s string) bool {
	t, err := clock.Parse(s)
	return err == nil && clock.Format(t) == s
}

// set is xs sorted with repeats removed, nil when empty.
func set(xs []string) []string {
	if len(xs) == 0 {
		return nil
	}
	s := append([]string(nil), xs...)
	sort.Strings(s)
	out := s[:1]
	for _, x := range s[1:] {
		if x != out[len(out)-1] {
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
