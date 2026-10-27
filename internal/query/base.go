package query

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mattjoyce/tarski"

	"github.com/mattjoyce/anam/internal/def"
	"github.com/mattjoyce/anam/internal/facet"
	"github.com/mattjoyce/anam/internal/fold"
)

// The base relations, read off a served value and the Definitions (ARCHITECTURE §9).
const (
	// RelEntry is entry(E, Type, FactTime, Series, Status, Source, DefinitionVersion):
	// one served facet's envelope. Series is "" for a facet that stands alone.
	RelEntry = "entry"
	// RelAttr is attr(E, Pointer, Value): every scalar leaf of a served payload, by
	// its JSON pointer. A null is no value; a container is not a value either, so
	// ask for its leaves.
	RelAttr = "attr"
	// RelCloses is closes(Type): the types whose Definition closes a series.
	RelCloses = "closes"
	// RelMonthOf is month_of(At, Month): the UTC month of each fact-time a served entry
	// carries, as "2006-01".
	RelMonthOf = "month_of"
	// RelDayOf is day_of(At, Day): the UTC day of each fact-time a served entry carries,
	// as "2006-01-02".
	RelDayOf = "day_of"
)

// ErrBase is returned when a served value cannot form the base relations.
var ErrBase = errors.New("query: base relations")

// Definitions is what the base relations and the pointer check need of the
// Definition set; *def.Registry satisfies it.
type Definitions interface {
	Closes(facet.Type) bool
	All() []def.Definition
}

// Schema names the base relations and their arities.
type Schema struct {
	Arities map[string]int
}

// BaseSchema is the schema every Base has.
var BaseSchema = Schema{Arities: map[string]int{RelEntry: 7, RelAttr: 3, RelCloses: 1, RelMonthOf: 2, RelDayOf: 2}}

// Versions is each live facet's definition_version by id, from fold.Result.Live at
// the watermark the value was served at. fold.Entry does not carry it: adding the
// field would move every checkpoint and served hash, so entry/7 joins the served
// entries back to the fold's facets instead.
func Versions(live []facet.Facet) map[string]int {
	out := make(map[string]int, len(live))
	for _, f := range live {
		out[f.ID] = f.DefinitionVersion
	}
	return out
}

// ReadsAttr is the one decision on whether a query needs attr/3 built: it reports
// whether any of roots, the relations the caller will read an answer from, reaches
// attr/3 through the rules of programs. It walks each root's rules and the rules of
// every relation their bodies read, negated or not. attr/3 is the base relation that
// costs a tuple per scalar leaf of every served payload, so a program that reads only
// entry/7 and closes/1 does not pay for it. A root that is attr itself reaches it. The
// bundled set reads attr in edge_to/2 and edge_from/2, so a program reaching those,
// or linked/2 through them, gets it. The walk is by relation name, so it errs toward
// building: a rule that mentions attr/3 anywhere along the way counts, even if no row
// of it could fire. Rules of relations no root reaches may read an unbuilt attr/3 and
// derive less than they would; they are not what the caller reads.
func ReadsAttr(roots []string, programs ...Program) bool {
	reads := bodyReads(programs)
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

// NewBase builds the base relations over served, a value a projection returned,
// with versions from the same fold (Versions). A served entry the fold does not hold
// is refused: the value and the versions must be from one watermark. attr/3 is built
// only when withAttr is set (ReadsAttr decides): left unbuilt it is an empty relation,
// and a rule that reads it derives nothing.
func NewBase(served fold.Value, versions map[string]int, defs Definitions, withAttr bool) (Base, error) {
	var facts []tarski.Fact
	seenTimes := map[string]bool{}
	for _, e := range served.Entries() {
		v, ok := versions[e.FacetID]
		if !ok {
			return Base{}, fmt.Errorf("%w: facet %s is in the served value but not in the fold it was served from", ErrBase, e.FacetID)
		}
		facts = append(facts, tarski.Fact{Relation: RelEntry, Tuple: Tuple{
			String(e.FacetID), String(string(e.Type)), String(e.FactTime),
			String(e.Series), String(string(e.Status)), String(e.Source), tarski.Int(int64(v)),
		}})
		if !seenTimes[e.FactTime] {
			seenTimes[e.FactTime] = true
			if len(e.FactTime) < 10 {
				return Base{}, fmt.Errorf("%w: facet %s: fact-time %q is not a lexeme", ErrBase, e.FacetID, e.FactTime)
			}
			facts = append(facts, tarski.Fact{Relation: RelMonthOf, Tuple: Tuple{String(e.FactTime), String(e.FactTime[:7])}})
			facts = append(facts, tarski.Fact{Relation: RelDayOf, Tuple: Tuple{String(e.FactTime), String(e.FactTime[:10])}})
		}
		if !withAttr {
			continue
		}
		doc, err := def.Decode(e.Payload)
		if err != nil {
			return Base{}, fmt.Errorf("%w: facet %s: %v", ErrBase, e.FacetID, err)
		}
		if err := leaves(doc, "", func(pointer string, c Const) {
			facts = append(facts, tarski.Fact{Relation: RelAttr, Tuple: Tuple{String(e.FacetID), String(pointer), c}})
		}); err != nil {
			return Base{}, fmt.Errorf("%w: facet %s: %v", ErrBase, e.FacetID, err)
		}
	}
	for _, d := range defs.All() {
		if defs.Closes(d.Type) {
			facts = append(facts, tarski.Fact{Relation: RelCloses, Tuple: Tuple{String(string(d.Type))}})
		}
	}
	// The base is built under a schema with no checks: the plan matches a base on arity
	// alone, and a check would run once per attr fact for nothing, since the facts come
	// from the fold and not from a program.
	b, err := tarski.NewBase(tarski.Schema{Arities: arities(BaseSchema)}, facts...)
	if err != nil {
		return Base{}, fmt.Errorf("%w: %w", ErrBase, err)
	}
	return b, nil
}

// leaves walks a decoded payload (def.Decode) and emits each scalar with its RFC
// 6901 pointer, object members in key order. It composes pointers; def.Resolve reads
// them back, and a test holds the two to agree on every leaf.
func leaves(doc any, prefix string, emit func(string, Const)) error {
	switch node := doc.(type) {
	case map[string]any:
		keys := make([]string, 0, len(node))
		for k := range node {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := leaves(node[k], prefix+"/"+escape(k), emit); err != nil {
				return err
			}
		}
	case []any:
		for i, item := range node {
			if err := leaves(item, fmt.Sprintf("%s/%d", prefix, i), emit); err != nil {
				return err
			}
		}
	case string:
		emit(prefix, String(node))
	case bool:
		emit(prefix, Bool(node))
	case nil:
		// A null is the absence of a value: no attr.
	default:
		// def.Decode preserves numbers as their lexemes.
		n, err := Number(fmt.Sprint(node))
		if err != nil {
			return err
		}
		emit(prefix, n)
	}
	return nil
}

func escape(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}
