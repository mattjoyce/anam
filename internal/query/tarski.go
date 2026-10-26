package query

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/mattjoyce/tarski"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/fold"
)

// This file is the second path, over the tarski module (card 230, slice 1): the same
// parse, compile, base and evaluate as the package's own engine, so a differential
// test can hold the two to the same answers and the same hashes before the swap. Nothing
// outside the package calls it yet.

// engineBuiltinsRefused are the builtins tarski brings that Anam's language does not
// have. The language stays identical in this card; a later one lifts the refusal.
var engineBuiltinsRefused = map[string]bool{"prefix": true, "suffix": true, "contains": true}

// tarskiSchema is the schema the plan is compiled under: the base relations, the
// attr/3 pointer check, and the within builtin. instant/1 is a derived relation, not a
// base one (the bundled set declares it and the query defines it), and tarski
// attaches a check only to a base relation, so checkInstants holds it instead.
func tarskiSchema(defs Definitions) tarski.Schema {
	return tarski.Schema{
		Arities: baseArities(),
		Checks: map[string]func(tarski.Atom) error{
			RelAttr: func(a tarski.Atom) error { return checkAttrTarski(a, defs) },
		},
		Builtins: tarskiBuiltins(),
	}
}

// tarskiBaseSchema is the schema a base is built under: the same relations and no
// checks. The plan matches a base on arity alone, and a check would run once per attr
// fact for nothing, since the facts come from the fold, not from a program.
func tarskiBaseSchema() tarski.Schema {
	return tarski.Schema{Arities: baseArities()}
}

func baseArities() map[string]int {
	out := make(map[string]int, len(BaseSchema.Arities))
	for name, n := range BaseSchema.Arities {
		out[name] = n
	}
	return out
}

// tarskiBuiltins is Anam's builtin table as tarski declares it.
func tarskiBuiltins() map[string]tarski.Builtin {
	out := make(map[string]tarski.Builtin, len(builtins))
	for _, b := range builtins {
		args := make([]tarski.BuiltinArg, len(b.Args))
		for i, a := range b.Args {
			args[i] = tarski.BuiltinArg{Name: a.Name, Doc: a.Doc}
		}
		out[b.Name] = tarski.Builtin{
			Args: args,
			Doc:  b.Doc,
			Check: func(consts []*tarski.Const) error {
				own := make([]*Const, len(consts))
				for i, c := range consts {
					if c == nil {
						continue
					}
					v, err := ownConst(*c)
					if err != nil {
						return err
					}
					own[i] = &v
				}
				return b.check(own)
			},
			Holds: func(args []tarski.Const) bool {
				own := make([]Const, len(args))
				for i, c := range args {
					v, err := ownConst(c)
					if err != nil {
						return false
					}
					own[i] = v
				}
				return b.holds(own)
			},
		}
	}
	return out
}

// ownConst reads a tarski constant as the package's own, through its canonical text,
// which the two engines write the same way.
func ownConst(c tarski.Const) (Const, error) {
	switch c.Kind() {
	case tarski.KindNumber:
		return Number(c.Text())
	case tarski.KindBool:
		return Bool(c.Text() == "true"), nil
	default:
		return String(c.Text()), nil
	}
}

// parseTarski reads a program with tarski's parser and refuses what Anam's language
// does not have: an aggregate, and the engine's string builtins.
func parseTarski(text string) (tarski.Program, error) {
	p, err := tarski.Parse(text)
	if err != nil {
		return tarski.Program{}, err
	}
	for _, r := range p.Rules {
		if engineBuiltinsRefused[r.Head.Name] {
			return tarski.Program{}, refusedBuiltin(r.Head.Name, r.Line)
		}
		for _, l := range r.Body {
			if l.Aggregate != nil {
				return tarski.Program{}, fmt.Errorf("%w: line %d: an aggregate is not part of Anam's language; "+
					"there is no count, sum, min or max, so count the answer's tuples instead (docs/QUERY.md)",
					tarski.ErrSyntax, l.Aggregate.Line)
			}
			if l.Atom != nil && engineBuiltinsRefused[l.Atom.Name] {
				return tarski.Program{}, refusedBuiltin(l.Atom.Name, l.Atom.Line)
			}
		}
	}
	for name := range p.Declared {
		if engineBuiltinsRefused[name] {
			return tarski.Program{}, refusedBuiltin(name, 0)
		}
	}
	return p, nil
}

func refusedBuiltin(name string, line int) error {
	return fmt.Errorf("%w: line %d: %s is not part of Anam's language; prefix, suffix and contains are the engine's "+
		"string builtins and are not offered, and the name is reserved", tarski.ErrInvalid, line, name)
}

// compileTarski validates programs under Anam's schema and refuses what the language
// does not have. The programs are read as one, the last being the query.
func compileTarski(defs Definitions, programs ...tarski.Program) (tarski.Plan, error) {
	plan, err := tarski.Compile(tarskiSchema(defs), programs...)
	if err != nil {
		return tarski.Plan{}, err
	}
	if plan.HasAggregates() {
		return tarski.Plan{}, fmt.Errorf("%w: an aggregate is not part of Anam's language; count the answer's tuples instead",
			tarski.ErrInvalid)
	}
	if err := checkInstants(programs); err != nil {
		return tarski.Plan{}, err
	}
	if err := checkDefinedTarski(plan, programs); err != nil {
		return tarski.Plan{}, err
	}
	return plan, nil
}

// checkInstants holds every constant in an instant/1 atom, head or body, to the
// fact-time lexeme. It is what Schema.Checks would do if tarski allowed a check on a
// derived relation.
func checkInstants(programs []tarski.Program) error {
	for _, p := range programs {
		for _, r := range p.Rules {
			atoms := []tarski.Atom{r.Head}
			for _, l := range r.Body {
				if l.Atom != nil {
					atoms = append(atoms, *l.Atom)
				}
			}
			for _, a := range atoms {
				if a.Name != RelInstant {
					continue
				}
				if err := checkInstantTarski(a); err != nil {
					return fmt.Errorf("%w: line %d: %v", tarski.ErrInvalid, a.Line, err)
				}
			}
		}
	}
	return nil
}

// checkDefinedTarski refuses a query whose rules reach, through any chain of rules, a
// declared relation nothing defines. Tarski reads such a relation as empty; Anam
// refuses it (docs/QUERY.md, Declared and undefined).
func checkDefinedTarski(plan tarski.Plan, programs []tarski.Program) error {
	if len(programs) == 0 {
		return nil
	}
	reads := map[string][]string{}
	for _, p := range programs {
		for _, r := range p.Rules {
			for _, l := range r.Body {
				if l.Atom != nil && !isOwnBuiltin(l.Atom.Name) {
					reads[r.Head.Name] = append(reads[r.Head.Name], l.Atom.Name)
				}
			}
		}
	}
	for _, r := range programs[len(programs)-1].Rules {
		if hole := reachTarski(plan, r.Head.Name, reads, map[string]bool{}); hole != "" {
			n, _ := plan.Arity(hole)
			return fmt.Errorf("%w: line %d: %s reads %s/%d, which is declared but has no rules or facts; define it in the query",
				tarski.ErrInvalid, r.Line, r.Head.Name, hole, n)
		}
	}
	return nil
}

func isOwnBuiltin(name string) bool {
	_, ok := builtinNamed(name)
	return ok
}

// reachTarski is the first undefined relation reachable from name, or "".
func reachTarski(plan tarski.Plan, name string, reads map[string][]string, seen map[string]bool) string {
	if seen[name] {
		return ""
	}
	seen[name] = true
	if BaseSchema.Arities[name] == 0 && !plan.Defines(name) {
		return name
	}
	for _, next := range reads[name] {
		if hole := reachTarski(plan, next, reads, seen); hole != "" {
			return hole
		}
	}
	return ""
}

// checkInstantTarski is checkInstant over tarski's atom.
func checkInstantTarski(a tarski.Atom) error {
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

// checkAttrTarski is the attr/3 pointer check over tarski's atom: a constant pointer
// is a string some Definition declares as a field.
func checkAttrTarski(a tarski.Atom, defs Definitions) error {
	if a.Args[1].IsVar() {
		return nil
	}
	p := a.Args[1].Const
	if p.Kind() != tarski.KindString {
		return fmt.Errorf("attr: the pointer must be a string, got %s", p.Text())
	}
	return checkPointer(p.Text(), defs)
}

// readsAttrTarski is ReadsAttr over tarski's programs.
func readsAttrTarski(roots []string, programs ...tarski.Program) bool {
	reads := map[string][]string{}
	for _, p := range programs {
		for _, r := range p.Rules {
			for _, l := range r.Body {
				if l.Atom != nil && !isOwnBuiltin(l.Atom.Name) {
					reads[r.Head.Name] = append(reads[r.Head.Name], l.Atom.Name)
				}
			}
		}
	}
	seen := map[string]bool{}
	stack := append([]string(nil), roots...)
	for len(stack) > 0 {
		name := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[name] {
			continue
		}
		seen[name] = true
		if name == RelAttr {
			return true
		}
		stack = append(stack, reads[name]...)
	}
	return false
}

// newBaseTarski builds the base relations over a served value as tarski facts, under a
// schema with no checks. It keeps ErrBase, the versions join and the lazy attr/3 of
// NewBase.
func newBaseTarski(served fold.Value, versions map[string]int, defs Definitions, withAttr bool) (tarski.Base, error) {
	var facts []tarski.Fact
	for _, e := range served.Entries() {
		v, ok := versions[e.FacetID]
		if !ok {
			return tarski.Base{}, fmt.Errorf("%w: facet %s is in the served value but not in the fold it was served from", ErrBase, e.FacetID)
		}
		facts = append(facts, tarski.Fact{Relation: RelEntry, Tuple: tarski.Tuple{
			tarski.String(e.FacetID), tarski.String(string(e.Type)), tarski.String(e.FactTime),
			tarski.String(e.Series), tarski.String(string(e.Status)), tarski.String(e.Source), tarski.Int(int64(v)),
		}})
		if !withAttr {
			continue
		}
		doc, err := def.Decode(e.Payload)
		if err != nil {
			return tarski.Base{}, fmt.Errorf("%w: facet %s: %v", ErrBase, e.FacetID, err)
		}
		var convErr error
		if err := leaves(doc, "", func(pointer string, c Const) {
			v, err := tarskiConst(c)
			if err != nil && convErr == nil {
				convErr = err
			}
			facts = append(facts, tarski.Fact{Relation: RelAttr, Tuple: tarski.Tuple{
				tarski.String(e.FacetID), tarski.String(pointer), v}})
		}); err != nil {
			return tarski.Base{}, fmt.Errorf("%w: facet %s: %v", ErrBase, e.FacetID, err)
		}
		if convErr != nil {
			return tarski.Base{}, fmt.Errorf("%w: facet %s: %v", ErrBase, e.FacetID, convErr)
		}
	}
	for _, d := range defs.All() {
		if defs.Closes(d.Type) {
			facts = append(facts, tarski.Fact{Relation: RelCloses, Tuple: tarski.Tuple{tarski.String(string(d.Type))}})
		}
	}
	b, err := tarski.NewBase(tarskiBaseSchema(), facts...)
	if err != nil {
		return tarski.Base{}, fmt.Errorf("%w: %w", ErrBase, err)
	}
	return b, nil
}

// tarskiConst reads the package's own constant as tarski's, through its canonical
// text. A number the package accepted and tarski refuses is an error, so the
// differential test shows it.
func tarskiConst(c Const) (tarski.Const, error) {
	switch c.Kind() {
	case KindNumber:
		return tarski.Number(c.Text())
	case KindBool:
		return tarski.Bool(c.Text() == "true"), nil
	default:
		return tarski.String(c.Text()), nil
	}
}

// runTarskiWith is RunWith over tarski: the rules' text and the query's text, parsed,
// compiled, evaluated within maxBytes and answered for relation.
func runTarskiWith(ctx context.Context, rulesText string, served fold.Value, versions map[string]int, defs Definitions,
	text, relation string, maxBytes int,
) (tarski.Answer, error) {
	rules, err := parseTarski(rulesText)
	if err != nil {
		return tarski.Answer{}, err
	}
	prog, err := parseTarski(text)
	if err != nil {
		return tarski.Answer{}, err
	}
	plan, err := compileTarski(defs, rules, prog)
	if err != nil {
		return tarski.Answer{}, err
	}
	base, err := newBaseTarski(served, versions, defs, readsAttrTarski([]string{relation}, rules, prog))
	if err != nil {
		return tarski.Answer{}, err
	}
	res, err := tarski.Evaluate(ctx, plan, base, tarski.Options{MaxBytes: maxBytes})
	if err != nil {
		return tarski.Answer{}, err
	}
	return res.Relation(relation)
}

// currentAtTarski is currentAt over tarski: the ids current at one instant, sorted.
func currentAtTarski(ctx context.Context, rules tarski.Program, base tarski.Base, defs Definitions, at string) ([]string, error) {
	prog, err := parseTarski(fmt.Sprintf("instant(%q).", at))
	if err != nil {
		return nil, err
	}
	plan, err := compileTarski(defs, rules, prog)
	if err != nil {
		return nil, err
	}
	res, err := tarski.Evaluate(ctx, plan, base, tarski.Options{})
	if err != nil {
		return nil, err
	}
	ans, err := res.Relation("current")
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, t := range ans.Tuples() {
		ids = append(ids, t[0].Text())
	}
	sort.Strings(ids)
	return ids, nil
}
