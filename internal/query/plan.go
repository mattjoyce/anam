package query

import (
	"fmt"
	"time"

	"github.com/mattjoyce/tarski"

	"github.com/mattjoyce/anam/internal/clock"
)

// The engine is the tarski module (github.com/mattjoyce/tarski, card 230). This
// package supplies what is Anam's: the base relations over a served value, the checks
// a constant is held to, within/3, the bundled rules, and the refusals that keep the
// language what it was.

// refusedNames are the builtins tarski brings that Anam's language does not have,
// and so names a program may not use. The language stays identical in card 230; a
// later card lifts the refusal.
var refusedNames = map[string]bool{"prefix": true, "suffix": true, "contains": true}

// Parse reads a program and refuses what Anam's language does not have: the names
// prefix, suffix and contains.
func Parse(text string) (Program, error) {
	p, err := tarski.Parse(text)
	if err != nil {
		return Program{}, err
	}
	for _, r := range p.Rules {
		if refusedNames[r.Head.Name] {
			return Program{}, refusedName(r.Head.Name, r.Line)
		}
		for _, l := range r.Body {
			for _, a := range bodyAtoms(l) {
				if refusedNames[a.Name] {
					return Program{}, refusedName(a.Name, a.Line)
				}
			}
		}
	}
	for name := range p.Declared {
		if refusedNames[name] {
			return Program{}, refusedName(name, 0)
		}
	}
	return p, nil
}

func refusedName(name string, line int) error {
	return fmt.Errorf("%w: line %d: %s is not part of Anam's language; prefix, suffix and contains are the engine's "+
		"string builtins and are not offered, and the name is reserved", ErrInvalid, line, name)
}

// schemaOf is the schema a plan is compiled under: the base relations, the attr/3
// pointer check, and the within builtin. instant/1 is a derived relation, not a base
// one (the bundled set declares it and the query defines it), and tarski attaches a
// check only to a base relation, so checkInstants holds it instead.
func schemaOf(schema Schema, defs Definitions) tarski.Schema {
	return tarski.Schema{
		Arities: arities(schema),
		Checks: map[string]func(Atom) error{
			RelAttr: func(a Atom) error { return checkAttr(a, defs) },
		},
		Builtins: tarskiBuiltins(),
	}
}

func arities(schema Schema) map[string]int {
	out := make(map[string]int, len(schema.Arities))
	for name, n := range schema.Arities {
		out[name] = n
	}
	return out
}

// Compile validates programs against a schema and groups their rules into strata.
// The programs are read as one: a later program defines what an earlier one declared,
// which is how a query builds on the bundled rule set. It also refuses what Anam's
// language does not have: a declared relation nothing defines, a constant instant that
// is not a fact-time.
func Compile(schema Schema, defs Definitions, programs ...Program) (Plan, error) {
	plan, err := tarski.Compile(schemaOf(schema, defs), programs...)
	if err != nil {
		return Plan{}, err
	}
	if err := checkInstants(programs); err != nil {
		return Plan{}, err
	}
	if err := checkDefined(plan, schema, programs); err != nil {
		return Plan{}, err
	}
	return plan, nil
}

// checkInstants holds every constant in an instant/1 atom, head or body, to the
// fact-time lexeme.
func checkInstants(programs []Program) error {
	for _, p := range programs {
		for _, r := range p.Rules {
			atoms := []Atom{r.Head}
			for _, l := range r.Body {
				for _, a := range bodyAtoms(l) {
					atoms = append(atoms, *a)
				}
			}
			for _, a := range atoms {
				if a.Name != RelInstant {
					continue
				}
				if err := checkInstant(a); err != nil {
					return fmt.Errorf("%w: line %d: %v", ErrInvalid, a.Line, err)
				}
			}
		}
	}
	return nil
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

// checkAttr holds a constant pointer in attr/3 to a string some Definition declares as
// a field.
func checkAttr(a Atom, defs Definitions) error {
	if a.Args[1].IsVar() {
		return nil
	}
	p := a.Args[1].Const
	if p.Kind() != KindString {
		return fmt.Errorf("attr: the pointer must be a string, got %s", p.Text())
	}
	return checkPointer(p.Text(), defs)
}

// checkDefined refuses a query whose rules reach, through any chain of rules, a
// declared relation nothing defines: the bundled current/2 without the instants to
// ask about, say. Tarski reads such a relation as empty. Reached silently it would be
// empty and the answer wrong; a query that never reaches it needs no definition.
func checkDefined(plan Plan, schema Schema, programs []Program) error {
	if len(programs) == 0 {
		return nil
	}
	reads := bodyReads(programs)
	for _, r := range programs[len(programs)-1].Rules {
		if hole := reach(plan, schema, r.Head.Name, reads, map[string]bool{}); hole != "" {
			n, _ := plan.Arity(hole)
			return fmt.Errorf("%w: line %d: %s reads %s/%d, which is declared but has no rules or facts; define it in the query",
				ErrInvalid, r.Line, r.Head.Name, hole, n)
		}
	}
	return nil
}

// bodyReads maps each derived relation to the relations its rules' bodies read,
// negated or not. A builtin is a guard, not a relation, and is left out.
func bodyReads(programs []Program) map[string][]string {
	reads := map[string][]string{}
	for _, p := range programs {
		for _, r := range p.Rules {
			for _, l := range r.Body {
				for _, a := range bodyAtoms(l) {
					if _, ok := builtinNamed(a.Name); !ok {
						reads[r.Head.Name] = append(reads[r.Head.Name], a.Name)
					}
				}
			}
		}
	}
	return reads
}

// bodyAtoms is every atom a body literal reads: the literal's own atom, or each atom
// inside an aggregate's braces.
func bodyAtoms(l Literal) []*Atom {
	if l.Aggregate != nil {
		out := make([]*Atom, 0, len(l.Aggregate.Body))
		for i := range l.Aggregate.Body {
			if l.Aggregate.Body[i].Atom != nil {
				out = append(out, l.Aggregate.Body[i].Atom)
			}
		}
		return out
	}
	if l.Atom != nil {
		return []*Atom{l.Atom}
	}
	return nil
}

// reach is the first undefined relation reachable from name, or "".
func reach(plan Plan, schema Schema, name string, reads map[string][]string, seen map[string]bool) string {
	if seen[name] {
		return ""
	}
	seen[name] = true
	if schema.Arities[name] == 0 && !plan.Defines(name) {
		return name
	}
	for _, next := range reads[name] {
		if hole := reach(plan, schema, next, reads, seen); hole != "" {
			return hole
		}
	}
	return ""
}
