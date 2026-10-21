package selection

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/mattjoyce/anam/internal/query"
)

// ErrNotChips is returned by Parse for a program that is not one Program writes: a
// join, a recursion, an attr, a hand-written rule of another shape, or text that is
// not a program at all. Such a program may still be a good one; it runs as text, and
// the criteria cannot be drawn from it.
var ErrNotChips = errors.New("selection: the program is not one the criteria make")

// Parse reads a program back to the filter it selects by, when the program is within
// the chip grammar: one that Program, given that filter, writes rule for rule. Layout
// and comments do not matter, nor does the order of the rules; anything else does,
// and the error says where the program leaves the grammar. Parse(f.Program()) is
// f.Canonical() for every filter Validate accepts.
func Parse(text string) (Filter, error) {
	prog, err := query.Parse(text)
	if err != nil {
		return Filter{}, fmt.Errorf("%w: %v", ErrNotChips, err)
	}
	for name, n := range prog.Declared {
		if name != Relation || n != 1 {
			return Filter{}, fmt.Errorf("%w: it declares %s/%d", ErrNotChips, name, n)
		}
	}
	f, err := extract(prog.Rules)
	if err != nil {
		return Filter{}, err
	}
	f = f.Canonical()
	if err := f.Validate(); err != nil {
		return Filter{}, fmt.Errorf("%w: %v", ErrNotChips, err)
	}
	if err := sameRules(prog, f); err != nil {
		return Filter{}, err
	}
	return f, nil
}

// extract reads a candidate filter off the rules: ticks and exclusions from the
// facts, criteria from the rules. It is lenient on purpose; sameRules then holds the
// candidate's own program to the text, which is where a program outside the grammar
// is caught.
func extract(rules []query.Rule) (Filter, error) {
	var f Filter
	criteria := false
	for _, r := range rules {
		if len(r.Body) == 0 {
			id, ok := factID(r)
			switch {
			case ok && r.Head.Name == Relation:
				f.Ticks = append(f.Ticks, id)
			case ok && r.Head.Name == Unpicked:
				f.Exclusions = append(f.Exclusions, id)
			default:
				return Filter{}, fmt.Errorf("%w: line %d: the fact %s is not a tick or an exclusion", ErrNotChips, r.Line, r.Head.Name)
			}
			continue
		}
		if r.Head.Name != Relation {
			return Filter{}, fmt.Errorf("%w: line %d: a rule for %s; the criteria write rules for %s alone", ErrNotChips, r.Line, r.Head.Name, Relation)
		}
		c, err := criteriaOf(r)
		if err != nil {
			return Filter{}, err
		}
		if criteria && (c.From != f.From || c.To != f.To || c.Standing != f.Standing) {
			return Filter{}, fmt.Errorf("%w: line %d: the rules disagree on the window or the standing", ErrNotChips, r.Line)
		}
		criteria = true
		f.From, f.To, f.Standing = c.From, c.To, c.Standing
		f.Types = append(f.Types, c.Types...)
		f.Sources = append(f.Sources, c.Sources...)
	}
	return f, nil
}

// factID is the one string constant of a unary fact.
func factID(r query.Rule) (string, bool) {
	if len(r.Head.Args) != 1 || r.Head.Args[0].IsVar() || r.Head.Args[0].Const.Kind() != query.KindString {
		return "", false
	}
	return r.Head.Args[0].Const.Text(), true
}

// criteriaOf reads one rule's criteria by the slot each comparison's variable stands
// in. What it cannot place it leaves out, for sameRules to find.
func criteriaOf(r query.Rule) (Filter, error) {
	first := r.Body[0].Atom
	if first == nil || r.Body[0].Negated || first.Name != query.RelEntry || len(first.Args) != entryArity {
		return Filter{}, fmt.Errorf("%w: line %d: a rule of the criteria opens with entry/7", ErrNotChips, r.Line)
	}
	slot := map[string]int{}
	for i, a := range first.Args {
		if a.IsVar() && a.Var != anonymous {
			slot[a.Var] = i
		}
	}
	var f Filter
	var lower string
	for _, l := range r.Body[1:] {
		c := l.Compare
		if c == nil || !c.Left.IsVar() || c.Right.IsVar() {
			continue
		}
		pos, ok := slot[c.Left.Var]
		if !ok {
			continue
		}
		v := c.Right.Const.Text()
		switch {
		case pos == typeSlot && c.Op == "=":
			f.Types = append(f.Types, v)
		case pos == typeSlot && c.Op == ">":
			lower = v
		case pos == typeSlot && c.Op == "<":
			f.Types = append(f.Types, lower)
		case pos == sourceSlot && c.Op == "=":
			f.Sources = append(f.Sources, v)
		case pos == atSlot && c.Op == ">=":
			f.From = v
		case pos == atSlot && c.Op == "<":
			f.To = v
		case pos == statusSlot:
			f.Standing = Superseded
		}
	}
	return f, nil
}

// sameRules holds the text's rules to the ones Program writes for f, as sets of
// rules written out one way, and names the first that differs.
func sameRules(prog query.Program, f Filter) error {
	text, err := f.Program()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotChips, err)
	}
	want, err := query.Parse(text)
	if err != nil {
		return fmt.Errorf("selection: the program written for the criteria does not parse: %w", err)
	}
	have := counts(prog.Rules)
	for k, n := range counts(want.Rules) {
		have[k] -= n
	}
	var extra []string
	for k, n := range have {
		if n != 0 {
			extra = append(extra, k)
		}
	}
	if len(extra) == 0 {
		return nil
	}
	sort.Strings(extra)
	for _, r := range prog.Rules {
		if have[ruleText(r)] > 0 {
			return fmt.Errorf("%w: line %d is not what the criteria write: %s", ErrNotChips, r.Line, ruleText(r))
		}
	}
	return fmt.Errorf("%w: it leaves out %s, which the criteria it reads as would write", ErrNotChips, extra[0])
}

func counts(rules []query.Rule) map[string]int {
	m := map[string]int{}
	for _, r := range rules {
		m[ruleText(r)]++
	}
	return m
}

// ruleText writes a rule on one line, constants quoted by kind, so two rules are the
// same rule exactly when their texts are equal.
func ruleText(r query.Rule) string {
	var b strings.Builder
	b.WriteString(atomText(r.Head))
	for i, l := range r.Body {
		if i == 0 {
			b.WriteString(" :- ")
		} else {
			b.WriteString(", ")
		}
		switch {
		case l.Compare != nil:
			b.WriteString(termText(l.Compare.Left) + " " + l.Compare.Op + " " + termText(l.Compare.Right))
		case l.Negated:
			b.WriteString("not " + atomText(*l.Atom))
		default:
			b.WriteString(atomText(*l.Atom))
		}
	}
	return b.String() + "."
}

func atomText(a query.Atom) string {
	args := make([]string, len(a.Args))
	for i, t := range a.Args {
		args[i] = termText(t)
	}
	return a.Name + "(" + strings.Join(args, ", ") + ")"
}

func termText(t query.Term) string {
	if t.IsVar() {
		return t.Var
	}
	if t.Const.Kind() == query.KindString {
		return strconv.Quote(t.Const.Text())
	}
	return t.Const.Text()
}
