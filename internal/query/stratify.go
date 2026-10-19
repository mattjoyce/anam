package query

import (
	"fmt"
	"sort"
)

// stratify groups rules into strata so that every negated relation is complete
// before a rule reads it: a relation's stratum is one above the highest it negates,
// and no less than any it depends on positively. A cycle through a negation has no
// such order and is refused.
func stratify(rules []Rule, idb map[string]bool) ([][]rule, error) {
	deps := dependencies(rules, idb)
	comps := components(deps)
	stratum := map[string]int{}
	for _, comp := range comps { // in reverse topological order: dependencies first
		members := map[string]bool{}
		for _, n := range comp {
			members[n] = true
		}
		level := 0
		for _, n := range comp {
			for _, d := range deps[n] {
				if members[d.to] {
					if d.negative {
						return nil, fmt.Errorf("%w: %s and %s depend on each other through a negation, so the program cannot be stratified; break the cycle",
							ErrInvalid, n, d.to)
					}
					continue
				}
				need := stratum[d.to]
				if d.negative {
					need++
				}
				if need > level {
					level = need
				}
			}
		}
		for _, n := range comp {
			stratum[n] = level
		}
	}
	return group(rules, stratum, idb), nil
}

type edge struct {
	to       string
	negative bool
}

// dependencies is the graph from each derived relation to the derived relations its
// rules read, marking an edge through a negation.
func dependencies(rules []Rule, idb map[string]bool) map[string][]edge {
	deps := map[string][]edge{}
	for _, r := range rules {
		if _, ok := deps[r.Head.Name]; !ok {
			deps[r.Head.Name] = nil
		}
		for _, l := range r.Body {
			if l.Atom != nil && idb[l.Atom.Name] {
				deps[r.Head.Name] = append(deps[r.Head.Name], edge{l.Atom.Name, l.Negated})
			}
		}
	}
	return deps
}

// components finds the strongly connected components of deps (Tarjan), returned
// with every component after the components it depends on. Nodes are visited in
// name order so the result is a function of the program, not of map order.
func components(deps map[string][]edge) [][]string {
	names := make([]string, 0, len(deps))
	for n := range deps {
		names = append(names, n)
	}
	sort.Strings(names)

	index, low := map[string]int{}, map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var out [][]string
	var visit func(string)
	visit = func(n string) {
		index[n], low[n] = len(index), len(index)
		stack = append(stack, n)
		onStack[n] = true
		for _, d := range deps[n] {
			if _, seen := index[d.to]; !seen {
				visit(d.to)
				low[n] = min(low[n], low[d.to])
			} else if onStack[d.to] {
				low[n] = min(low[n], index[d.to])
			}
		}
		if low[n] != index[n] {
			return
		}
		var comp []string
		for {
			m := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[m] = false
			comp = append(comp, m)
			if m == n {
				break
			}
		}
		sort.Strings(comp)
		out = append(out, comp)
	}
	for _, n := range names {
		if _, seen := index[n]; !seen {
			visit(n)
		}
	}
	return out
}

// group orders each rule's body for evaluation and places it in its head's stratum.
func group(rules []Rule, stratum map[string]int, idb map[string]bool) [][]rule {
	top := 0
	for _, s := range stratum {
		top = max(top, s)
	}
	strata := make([][]rule, top+1)
	for _, r := range rules {
		s := stratum[r.Head.Name]
		pr := rule{Rule: r, ordered: order(r.Body)}
		for i, l := range pr.ordered {
			if l.Atom != nil && !l.Negated && idb[l.Atom.Name] && stratum[l.Atom.Name] == s {
				pr.recursive = append(pr.recursive, i)
			}
		}
		strata[s] = append(strata[s], pr)
	}
	return strata
}

// order places each negation and comparison right after the positive atom that
// binds its last variable, so a guard runs as soon as it can and never before it
// may. Positive atoms keep the order written.
func order(body []Literal) []Literal {
	var out []Literal
	bound := map[string]bool{}
	var pending []Literal
	for _, l := range body {
		if l.Atom == nil || l.Negated {
			pending = append(pending, l)
		}
	}
	flush := func() {
		var still []Literal
		for _, l := range pending {
			if ready(l, bound) {
				out = append(out, l)
			} else {
				still = append(still, l)
			}
		}
		pending = still
	}
	flush()
	for _, l := range body {
		if l.Atom == nil || l.Negated {
			continue
		}
		out = append(out, l)
		for _, a := range l.Atom.Args {
			if a.IsVar() {
				bound[a.Var] = true
			}
		}
		flush()
	}
	return append(out, pending...) // unreachable after checkRule; kept so nothing is dropped
}

func ready(l Literal, bound map[string]bool) bool {
	var terms []Term
	if l.Atom != nil {
		terms = l.Atom.Args
	} else {
		terms = []Term{l.Compare.Left, l.Compare.Right}
	}
	for _, t := range terms {
		if t.IsVar() && t.Var != "_" && !bound[t.Var] {
			return false
		}
	}
	return true
}
