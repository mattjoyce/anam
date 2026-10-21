package query

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
)

// ErrInvalid is returned for a program that parses but cannot run: an unknown
// relation, a wrong arity, an unsafe variable, a negation that cannot be stratified,
// a rule over a base relation, or a pointer no Definition declares. Each message says
// what to change.
var ErrInvalid = errors.New("query: invalid program")

// Plan is a validated, stratified program over a schema, ready to evaluate.
type Plan struct {
	strata [][]rule
	idb    map[string]bool
}

// rule is a Rule with its body in evaluation order: positive atoms as written, each
// followed by the negations and comparisons it binds, and with the positions of the
// positive atoms whose relation the rule's own stratum derives.
type rule struct {
	Rule
	ordered   []Literal
	recursive []int
}

// Compile validates programs against a schema and groups their rules into strata.
// The programs are read as one: a later program defines what an earlier one declared,
// which is how a query builds on the bundled rule set.
func Compile(schema Schema, defs Definitions, programs ...Program) (Plan, error) {
	arities, idb, err := relations(schema, programs)
	if err != nil {
		return Plan{}, err
	}
	var rules []Rule
	for _, p := range programs {
		for _, r := range p.Rules {
			if err := checkRule(r, arities, defs); err != nil {
				return Plan{}, err
			}
			rules = append(rules, r)
		}
	}
	if err := checkDefined(programs, rules, arities, schema); err != nil {
		return Plan{}, err
	}
	strata, err := stratify(rules, idb)
	if err != nil {
		return Plan{}, err
	}
	return Plan{strata: strata, idb: idb}, nil
}

// relations collects every relation's arity: the schema's base relations, the
// declarations, and the heads. One name, one arity.
func relations(schema Schema, programs []Program) (arities map[string]int, idb map[string]bool, err error) {
	arities, idb = map[string]int{}, map[string]bool{}
	for name, n := range schema.Arities {
		arities[name] = n
	}
	note := func(name string, n, line int) error {
		if have, ok := arities[name]; ok && have != n {
			return fmt.Errorf("%w: line %d: %s/%d, but %s is %s/%d", ErrInvalid, line, name, n, name, name, have)
		}
		arities[name] = n
		return nil
	}
	for _, p := range programs {
		for name, n := range p.Declared {
			if schema.Arities[name] != 0 {
				return nil, nil, fmt.Errorf("%w: %s/%d is a base relation and cannot be declared", ErrInvalid, name, n)
			}
			if err := note(name, n, 0); err != nil {
				return nil, nil, err
			}
			idb[name] = true
		}
		for _, r := range p.Rules {
			if schema.Arities[r.Head.Name] != 0 {
				return nil, nil, fmt.Errorf("%w: line %d: %s/%d is a base relation; a rule cannot add to it",
					ErrInvalid, r.Line, r.Head.Name, schema.Arities[r.Head.Name])
			}
			if err := note(r.Head.Name, len(r.Head.Args), r.Line); err != nil {
				return nil, nil, err
			}
			idb[r.Head.Name] = true
		}
	}
	return arities, idb, nil
}

// checkRule holds one rule to the arities, to safety, and to the Definitions: every
// variable in the head, in a negated atom or in a comparison is bound by a positive
// body atom, and a constant pointer in attr/3 is a field some Definition declares.
func checkRule(r Rule, arities map[string]int, defs Definitions) error {
	bound := map[string]bool{}
	atoms := []*Atom{&r.Head}
	for _, l := range r.Body {
		if l.Atom != nil {
			atoms = append(atoms, l.Atom)
			if !l.Negated {
				for _, a := range l.Atom.Args {
					if a.IsVar() {
						bound[a.Var] = true
					}
				}
			}
		}
	}
	for _, a := range atoms {
		if err := checkAtom(*a, arities, defs); err != nil {
			return fmt.Errorf("%w: line %d: %v", ErrInvalid, a.Line, err)
		}
	}
	for _, a := range r.Head.Args {
		if a.Var == "_" {
			return fmt.Errorf("%w: line %d: _ in the head of %s; name the variable or use a constant", ErrInvalid, r.Line, r.Head.Name)
		}
		if a.IsVar() && !bound[a.Var] {
			return fmt.Errorf("%w: line %d: %s in the head of %s is not bound by a positive body atom", ErrInvalid, r.Line, a.Var, r.Head.Name)
		}
	}
	for _, l := range r.Body {
		switch {
		case l.Negated:
			for _, a := range l.Atom.Args {
				if a.IsVar() && a.Var != "_" && !bound[a.Var] {
					return fmt.Errorf("%w: line %d: %s in not %s is not bound by a positive body atom", ErrInvalid, l.Atom.Line, a.Var, l.Atom.Name)
				}
			}
		case l.Compare != nil:
			for _, t := range []Term{l.Compare.Left, l.Compare.Right} {
				if t.Var == "_" {
					return fmt.Errorf("%w: line %d: _ in a comparison compares nothing", ErrInvalid, l.Compare.Line)
				}
				if t.IsVar() && !bound[t.Var] {
					return fmt.Errorf("%w: line %d: %s in a comparison is not bound by a positive body atom", ErrInvalid, l.Compare.Line, t.Var)
				}
			}
		}
	}
	return nil
}

func checkAtom(a Atom, arities map[string]int, defs Definitions) error {
	n, ok := arities[a.Name]
	if !ok {
		return fmt.Errorf("unknown relation %s/%d; the relations are %s", a.Name, len(a.Args), known(arities))
	}
	if n != len(a.Args) {
		return fmt.Errorf("%s has %d arguments, but %s is %s/%d", a.Name, len(a.Args), a.Name, a.Name, n)
	}
	if a.Name == RelInstant {
		return checkInstant(a)
	}
	if a.Name != RelAttr || a.Args[1].IsVar() {
		return nil
	}
	p := a.Args[1].Const
	if p.Kind() != KindString {
		return fmt.Errorf("attr: the pointer must be a string, got %s", p.Text())
	}
	return checkPointer(p.Text(), defs)
}

// checkInstant holds a constant in instant/1 to the fact-time lexeme (clock.Layout):
// a bare date is refused with the lexeme it should have been, since as written it
// sorts before every entry of its day.
func checkInstant(a Atom) error {
	for _, arg := range a.Args {
		if arg.IsVar() {
			continue
		}
		s := arg.Const.Text()
		if t, err := clock.Parse(s); err == nil && clock.Format(t) == s {
			continue
		}
		example := "2026-06-15T00:00:00.000000000Z"
		if day, err := time.Parse(time.DateOnly, s); err == nil {
			example = clock.Format(day)
		}
		return fmt.Errorf("instant %s is not a fact-time; write the whole instant, as %s (layout %s)", arg.Const.Text(), example, clock.Layout)
	}
	return nil
}

// checkPointer is the fields' own check (card 138): the pointer is well formed and
// some Definition declares a field there. The types a value holds decide which
// Definition answers a given facet; a pointer no type declares cannot answer for any.
//
// A pointer that names a container is refused too, when every Definition that
// declares it declares it as one: attr holds the leaves of a payload, and a
// container holds none, so the program would answer empty forever and say nothing.
// The refusal names the leaves to ask for instead.
func checkPointer(p string, defs Definitions) error {
	if err := def.CheckPointer(p); err != nil {
		return fmt.Errorf("attr: %w", err)
	}
	var (
		declared  int
		leaves    []string
		container = true
	)
	for _, d := range defs.All() {
		if d.HasPointer(p) != nil {
			continue
		}
		declared++
		isContainer, l, err := d.ContainerAt(p)
		if err != nil {
			return fmt.Errorf("attr: %w", err)
		}
		if !isContainer {
			container = false
			continue
		}
		leaves = append(leaves, l...)
	}
	if declared == 0 {
		var types []string
		for _, d := range defs.All() {
			types = append(types, string(d.Type))
		}
		return fmt.Errorf("attr: no Definition declares a field at %s (the types are %s)", p, strings.Join(types, ", "))
	}
	if !container {
		return nil
	}
	leaves = uniq(leaves)
	if len(leaves) == 0 {
		return fmt.Errorf("attr: %s names a container, not a field, and the schema does not say what is in it", p)
	}
	return fmt.Errorf("attr: %s names a container, not a field; ask for its leaves: %s", p, strings.Join(leaves, ", "))
}

// uniq sorts and removes the duplicates, so a leaf several Definitions declare is
// named once.
func uniq(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	sort.Strings(in)
	out := in[:1]
	for _, s := range in[1:] {
		if s != out[len(out)-1] {
			out = append(out, s)
		}
	}
	return out
}

// checkDefined refuses a query whose rules reach, through any chain of rules, a
// declared relation nothing defines: the bundled current/2 without the instants to
// ask about, say. Reached silently it would be empty and the answer wrong; a query
// that never reaches it needs no definition.
func checkDefined(programs []Program, rules []Rule, arities map[string]int, schema Schema) error {
	defined, reads := map[string]bool{}, bodyReads(rules)
	for _, r := range rules {
		defined[r.Head.Name] = true
	}
	if len(programs) == 0 {
		return nil
	}
	for _, r := range programs[len(programs)-1].Rules {
		seen := map[string]bool{}
		if hole := reach(r.Head.Name, reads, defined, schema, seen); hole != "" {
			return fmt.Errorf("%w: line %d: %s reads %s/%d, which is declared but has no rules or facts; define it in the query",
				ErrInvalid, r.Line, r.Head.Name, hole, arities[hole])
		}
	}
	return nil
}

// bodyReads maps each derived relation to the relations its rules' bodies read,
// negated or not.
func bodyReads(rules []Rule) map[string][]string {
	reads := map[string][]string{}
	for _, r := range rules {
		for _, l := range r.Body {
			if l.Atom != nil {
				reads[r.Head.Name] = append(reads[r.Head.Name], l.Atom.Name)
			}
		}
	}
	return reads
}

// reach is the first undefined relation reachable from name, or "".
func reach(name string, reads map[string][]string, defined map[string]bool, schema Schema, seen map[string]bool) string {
	if seen[name] {
		return ""
	}
	seen[name] = true
	if schema.Arities[name] == 0 && !defined[name] {
		return name
	}
	for _, next := range reads[name] {
		if hole := reach(next, reads, defined, schema, seen); hole != "" {
			return hole
		}
	}
	return ""
}

func known(arities map[string]int) string {
	names := make([]string, 0, len(arities))
	for name, n := range arities {
		names = append(names, fmt.Sprintf("%s/%d", name, n))
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}
