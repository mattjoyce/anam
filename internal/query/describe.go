package query

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/anam/internal/clock"
	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
)

// Introspection (card 173): what an agent reads before writing a query. The
// relations, the attributes each type exposes at each Definition version, the exact
// instant lexeme, and the bundled rules, all from the Definitions and the rule set
// alone. It reads no facet.

// Relation describes one relation: its name, arity, argument names and a line of
// prose. Kind is base, parameter or rule.
type Relation struct {
	Name  string   `json:"name"`
	Arity int      `json:"arity"`
	Args  []string `json:"args"`
	Kind  string   `json:"kind"`
	Doc   string   `json:"doc,omitempty"`
}

// InstantLexeme is how a fact-time is written: the layout and one example.
type InstantLexeme struct {
	Layout  string `json:"layout"`
	Example string `json:"example"`
}

// VersionAttributes lists the pointers one Definition version declares.
type VersionAttributes struct {
	Version  int      `json:"version"`
	Pointers []string `json:"pointers"`
}

// TypeAttributes is one type's attributes: its current version, its series family
// and whether it closes one, and the pointers each held version declares. A
// pointer in some versions and not others reaches only the facets admitted under
// the versions that declare it.
type TypeAttributes struct {
	Type     facet.Type          `json:"type"`
	Current  int                 `json:"current"`
	Series   string              `json:"series,omitempty"`
	Closes   bool                `json:"closes"`
	Versions []VersionAttributes `json:"versions"`
}

// RuleSetInfo describes the bundled rule set: identity, hash, the text itself and
// the relations it defines.
type RuleSetInfo struct {
	Name      string     `json:"name"`
	Version   int        `json:"version"`
	Hash      string     `json:"hash"`
	Text      string     `json:"text"`
	Relations []Relation `json:"relations"`
}

// Catalogue is the whole introspection answer.
type Catalogue struct {
	Syntax    string           `json:"syntax"`
	Relations []Relation       `json:"relations"`
	Instant   InstantLexeme    `json:"instant"`
	Types     []TypeAttributes `json:"types"`
	Rules     RuleSetInfo      `json:"rules"`
}

// Syntax is the language in a paragraph, for an agent about to write a program.
const Syntax = `A program is rules, facts and declarations, each ending in a period; % starts a comment.
  head(X, Y) :- rel(X, Z), not other(Z), Y > 3.   a rule: head holds when every body literal does
  fact("x", 1).                                   a fact
  name/2.                                         declares a relation a later program defines
Relation names are lower-case, variables start upper-case, _ matches anything. Constants are
"strings", numbers and true/false. Comparisons: = != < <= > >=; strings compare lexically, so
fact-times compare chronologically; numbers numerically; different kinds are unequal and unordered.
Every variable in a head, a negation or a comparison must appear in a positive body atom; negation
may not cycle. The answer is one relation, named with the query. current/2 and displaced/2 need
instant/1 defined, as facts with the whole fact-time lexeme or as a rule over entry/7.`

// Held is every Definition body Anam holds, by type and version: what the daemon
// serves at /v1/definitions and def.Manifests carries.
type Held map[facet.Type]map[int][]byte

// HeldBundled is the bundled manifests and their history as a Held.
func HeldBundled() (Held, error) {
	stored, err := def.Manifests()
	if err != nil {
		return nil, err
	}
	held := Held{}
	for _, s := range stored {
		if held[s.Type] == nil {
			held[s.Type] = map[int][]byte{}
		}
		held[s.Type][s.Version] = s.Body
	}
	return held, nil
}

// baseRelations is the schema in words.
var baseRelations = []Relation{
	{Name: RelEntry, Arity: 7, Kind: "base", Args: []string{"E", "Type", "FactTime", "Series", "Status", "Source", "DefinitionVersion"},
		Doc: "one served facet's envelope: id, type, fact-time, series (\"\" for none), status, source, and the Definition version it was admitted under"},
	{Name: RelAttr, Arity: 3, Kind: "base", Args: []string{"E", "Pointer", "Value"},
		Doc: "every scalar leaf of a served payload by its JSON pointer; a null is no value and a container is not a value either, so ask for the leaves beneath it; a constant pointer must be one some Definition declares and not one that names a container"},
	{Name: RelCloses, Arity: 1, Kind: "base", Args: []string{"Type"},
		Doc: "the types whose Definition closes a series"},
	{Name: RelInstant, Arity: 1, Kind: "parameter", Args: []string{"At"},
		Doc: "the fact-times a query asks about; the query defines it, as facts or a rule"},
}

// Describe builds the catalogue from the Definitions in force, every held body, and
// the bundled rule set.
func Describe(defs Definitions, held Held) (Catalogue, error) {
	rules, err := Bundled()
	if err != nil {
		return Catalogue{}, err
	}
	cat := Catalogue{
		Syntax:    Syntax,
		Relations: baseRelations,
		Instant:   InstantLexeme{Layout: clock.Layout, Example: "2026-06-15T00:00:00.000000000Z"},
		Rules:     RuleSetInfo{Name: rules.Name, Version: rules.Version, Hash: rules.Hash, Text: rules.Program.Text},
	}
	cat.Rules.Relations = ruleRelations(rules.Program)
	for _, d := range defs.All() {
		ta := TypeAttributes{Type: d.Type, Current: d.Version, Closes: defs.Closes(d.Type)}
		if d.Series != nil {
			ta.Series = d.Series.Name
		}
		versions := held[d.Type]
		if _, ok := versions[d.Version]; !ok {
			ta.Versions = append(ta.Versions, VersionAttributes{Version: d.Version, Pointers: pointersOrEmpty(d.Pointers())})
		}
		for v, body := range versions {
			ps, err := def.SchemaPointers(body)
			if err != nil {
				return Catalogue{}, fmt.Errorf("query: %s v%d: %w", d.Type, v, err)
			}
			ta.Versions = append(ta.Versions, VersionAttributes{Version: v, Pointers: pointersOrEmpty(ps)})
		}
		sort.Slice(ta.Versions, func(i, j int) bool { return ta.Versions[i].Version < ta.Versions[j].Version })
		cat.Types = append(cat.Types, ta)
	}
	return cat, nil
}

func pointersOrEmpty(ps []string) []string {
	if ps == nil {
		return []string{}
	}
	return ps
}

// ruleRelations lists the relations a program defines, with the argument names of
// the first rule for each and the comment block above it as its doc.
func ruleRelations(p Program) []Relation {
	docs := docsOf(p.Text)
	var out []Relation
	seen := map[string]bool{}
	for _, r := range p.Rules {
		if seen[r.Head.Name] {
			continue
		}
		seen[r.Head.Name] = true
		rel := Relation{Name: r.Head.Name, Arity: len(r.Head.Args), Kind: "rule", Doc: docs[r.Head.Name]}
		for _, a := range r.Head.Args {
			if a.IsVar() {
				rel.Args = append(rel.Args, a.Var)
			} else {
				rel.Args = append(rel.Args, a.Const.Text())
			}
		}
		out = append(out, rel)
	}
	return out
}

// docsOf reads the comment block directly above each relation's first statement.
func docsOf(text string) map[string]string {
	docs := map[string]string{}
	var block []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "%"):
			block = append(block, strings.TrimSpace(strings.TrimPrefix(line, "%")))
		case line == "":
			block = nil
		default:
			name := line
			if i := strings.IndexAny(line, "(/"); i > 0 {
				name = line[:i]
			}
			if _, ok := docs[name]; !ok && len(block) > 0 {
				docs[name] = strings.Join(block, " ")
			}
			block = nil
		}
	}
	return docs
}

// PointerWarnings lists, for each constant pointer in an attr/3 atom of programs,
// the types that declare it at some held versions and not others: facets admitted
// under the versions that lack it have no attr for it, and a query that ignores the
// Definition version misses them silently.
func PointerWarnings(programs []Program, held Held) ([]string, error) {
	pointers := map[string]bool{}
	for _, p := range programs {
		for _, r := range p.Rules {
			for _, l := range r.Body {
				if l.Atom != nil && l.Atom.Name == RelAttr && !l.Atom.Args[1].IsVar() {
					pointers[l.Atom.Args[1].Const.Text()] = true
				}
			}
		}
	}
	var warnings []string
	for _, p := range sortedKeys(pointers) {
		for _, t := range sortedTypes(held) {
			var with, without []string
			for _, v := range sortedVersions(held[t]) {
				ps, err := def.SchemaPointers(held[t][v])
				if err != nil {
					return nil, fmt.Errorf("query: %s v%d: %w", t, v, err)
				}
				if declares(ps, p) {
					with = append(with, fmt.Sprintf("v%d", v))
				} else {
					without = append(without, fmt.Sprintf("v%d", v))
				}
			}
			if len(with) > 0 && len(without) > 0 {
				warnings = append(warnings, fmt.Sprintf("%s declares %s at %s and not at %s; facets admitted under those versions have no attr for it",
					t, p, strings.Join(with, ", "), strings.Join(without, ", ")))
			}
		}
	}
	return warnings, nil
}

func declares(pointers []string, p string) bool {
	for _, q := range pointers {
		if q == p {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedTypes(h Held) []facet.Type {
	out := make([]facet.Type, 0, len(h))
	for t := range h {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sortedVersions(m map[int][]byte) []int {
	out := make([]int, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}
