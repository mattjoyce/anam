package selection

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The program a filter compiles to. It defines pick/1, the facets selected, over the
// query surface's entry/7 (internal/query): one rule per pair of a type and a source
// the filter names, each a conjunction of the criteria, so the rules' union is the
// filter's "any of these types, any of these sources". A tick is a fact,
// pick("<id>"); an exclusion is a fact, unpick("<id>"), that each rule reads as
// not unpick(E). A prefix is a pair of lexical bounds, as the bundled edge rules
// bound a pointer; the program reads no attr, so it never names a pointer.
const (
	// Relation is the unary relation a selection program defines: the facets it
	// selects. A consumer answers it (POST /v1/query's relation).
	Relation = "pick"
	// Unpicked is the relation of the exclusions.
	Unpicked = "unpick"
)

// header opens every program: what it is, and the declaration that makes pick/1 a
// relation even when nothing is selected, so an empty selection answers empty.
const header = "% An Anam selection (card 199): pick/1 holds the facets it selects.\n" + Relation + "/1.\n"

// The variables of the rules' entry atom, one per slot a criterion reads.
const (
	varFacet    = "E"
	varType     = "T"
	varAt       = "At"
	varStatus   = "St"
	varSource   = "S"
	anonymous   = "_"
	statusSlot  = 4
	sourceSlot  = 5
	entryArity  = 7
	typeSlot    = 1
	atSlot      = 2
	prefixUpper = "/" // the byte after ".", so a prefix's upper bound is the prefix with "." made "/"
)

// ZoneComment opens the comment line that names the zone a program's dates were read
// in (card 236): "% dates read in Australia/Sydney". The bounds are instants and mean
// the same in any zone; the comment says what "12 Oct" meant to the person who chose
// it. Parse ignores it, as it ignores every comment.
const ZoneComment = "% dates read in "

// Program is the filter as Datalog text, in one canonical layout: Parse reads it back
// to the canonical filter. It refuses a filter Validate refuses. It names no zone;
// ProgramIn does.
func (f Filter) Program() (string, error) { return f.ProgramIn("") }

// ProgramIn is Program with the zone the dates were read in, as a comment line after
// the header, when the filter has a bound and the zone is one ZoneOf would read back.
// A filter without a bound read no date, and a zone that is not an IANA name is not
// written.
func (f Filter) ProgramIn(zone string) (string, error) {
	if err := f.Validate(); err != nil {
		return "", err
	}
	c := f.Canonical()
	var b strings.Builder
	b.WriteString(header)
	if (c.From != "" || c.To != "") && validZone(zone) {
		b.WriteString(ZoneComment + zone + "\n")
	}
	if c.HasCriteria() {
		for _, typ := range orNone(c.Types) {
			for _, src := range orNone(c.Sources) {
				b.WriteString(c.rule(typ, src))
			}
		}
	}
	for _, id := range c.Ticks {
		fmt.Fprintf(&b, "%s(%s).\n", Relation, strconv.Quote(id))
	}
	for _, id := range c.Exclusions {
		fmt.Fprintf(&b, "%s(%s).\n", Unpicked, strconv.Quote(id))
	}
	return b.String(), nil
}

// ZoneOf is the zone a program says its dates were read in: the name on its first
// ZoneComment line, when it is an IANA name this machine knows. "" for a program that
// names none, such as one saved before the comment existed, which read its days in UTC.
func ZoneOf(program string) string {
	for _, line := range strings.Split(program, "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), ZoneComment); ok {
			name = strings.TrimSpace(name)
			if validZone(name) {
				return name
			}
			return ""
		}
	}
	return ""
}

// validZone reports whether name is an IANA zone: one that loads, and not Local, which
// names whatever machine reads it.
func validZone(name string) bool {
	if name == "" || name == "Local" {
		return false
	}
	_, err := time.LoadLocation(name)
	return err == nil
}

// orNone is xs, or one empty value for a criterion the filter does not name.
func orNone(xs []string) []string {
	if len(xs) == 0 {
		return []string{""}
	}
	return xs
}

// rule is one conjunction: entry(...) with a variable in each slot a criterion reads,
// then the type, the source, the window, the standing and the exclusions, one group
// to a line.
func (f Filter) rule(typ, src string) string {
	slots := []string{varFacet, anonymous, anonymous, anonymous, anonymous, anonymous, anonymous}
	var groups []string
	if typ != "" {
		slots[typeSlot] = varType
		groups = append(groups, typeLiterals(typ))
	}
	if src != "" {
		slots[sourceSlot] = varSource
		groups = append(groups, compare(varSource, "=", src))
	}
	var window []string
	if f.From != "" {
		window = append(window, compare(varAt, ">=", f.From))
	}
	if f.To != "" {
		window = append(window, compare(varAt, "<", f.To))
	}
	if len(window) > 0 {
		slots[atSlot] = varAt
		groups = append(groups, strings.Join(window, ", "))
	}
	if f.Standing == Superseded {
		slots[statusSlot] = varStatus
		groups = append(groups, compare(varStatus, "!=", "admitted")+", "+compare(varStatus, "!=", "endorsed"))
	}
	if len(f.Exclusions) > 0 {
		groups = append(groups, fmt.Sprintf("not %s(%s)", Unpicked, varFacet))
	}
	head := fmt.Sprintf("%s(%s) :- entry(%s)", Relation, varFacet, strings.Join(slots, ", "))
	return head + ",\n    " + strings.Join(groups, ",\n    ") + ".\n"
}

// typeLiterals is an exact type's equality, or a prefix's lexical bounds: above the
// prefix and below it with its closing "." raised to "/", which is every string that
// starts with the prefix and is longer, facet.Matches's own rule.
func typeLiterals(p string) string {
	if !strings.HasSuffix(p, ".") {
		return compare(varType, "=", p)
	}
	upper := strings.TrimSuffix(p, ".") + prefixUpper
	return compare(varType, ">", p) + ", " + compare(varType, "<", upper)
}

func compare(v, op, c string) string { return v + " " + op + " " + strconv.Quote(c) }
