package query

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/mattjoyce/anam/internal/clock"
)

// A builtin is a predicate the evaluator answers in Go, because the language has no
// arithmetic to write it in (card 212). It reads like an atom in a body, but it names
// no relation: it is a guard, as a comparison is, true or false of constants already
// bound. The plan check refuses a builtin whose required argument is not bound by a
// positive body atom, and the evaluator tests it where a comparison is tested. The
// stratifier sees no relation in it.
//
// The table is where the next builtin goes (card 213's month and day functions): a new
// entry, not a new case in the evaluator.

// BuiltinArg is one argument of a builtin: its name, whether the program must bind it
// before the builtin is read, and what it means.
type BuiltinArg struct {
	Name  string `json:"name"`
	Bound bool   `json:"bound"`
	Doc   string `json:"doc"`
}

// Builtin is one entry of the table.
type Builtin struct {
	Name string       `json:"name"`
	Args []BuiltinArg `json:"args"`
	Doc  string       `json:"doc"`

	// check refuses a constant argument that can never be right, at plan time. A
	// position that is a variable is nil in consts; it is judged when it is bound.
	check func(consts []*Const) error
	// holds is the predicate over the bound arguments. A value of the wrong kind or
	// shape is false, not an error: it came from data, and the data is not the
	// program's fault.
	holds func(args []Const) bool
}

// withinDoc is the contract the catalogue prints, kept beside the function.
const withinDoc = "true when the fact-times A and B are at most Days days apart, in either order. " +
	"A and B are whole fact-time lexemes; Days is a number that is not negative and may be a decimal " +
	"(0.25 is six hours). A day is 24 hours of UTC, since a fact-time is one UTC instant. " +
	"All three arguments must be bound by a positive body atom"

// builtins is the table, in the order the catalogue lists it.
var builtins = []Builtin{
	{
		Name: "within",
		Args: []BuiltinArg{
			{Name: "A", Bound: true, Doc: "a fact-time lexeme"},
			{Name: "B", Bound: true, Doc: "a fact-time lexeme"},
			{Name: "Days", Bound: true, Doc: "a number of days, not negative; a decimal is allowed"},
		},
		Doc:   withinDoc,
		check: checkWithin,
		holds: holdsWithin,
	},
}

// Builtins is the table, for the catalogue and for a client that lists them.
func Builtins() []Builtin { return append([]Builtin(nil), builtins...) }

// builtinNamed finds a builtin by name.
func builtinNamed(name string) (Builtin, bool) {
	for _, b := range builtins {
		if b.Name == name {
			return b, true
		}
	}
	return Builtin{}, false
}

// isBuiltin reports whether l is a builtin literal, negated or not.
func (l Literal) isBuiltin() bool {
	if l.Atom == nil {
		return false
	}
	_, ok := builtinNamed(l.Atom.Name)
	return ok
}

// guard reports whether l only tests bound values: a negation, a comparison or a
// builtin. A positive atom over a relation is the only literal that binds.
func (l Literal) guard() bool { return l.Atom == nil || l.Negated || l.isBuiltin() }

// signature is the builtin as a program writes it, within(A, B, Days).
func (b Builtin) signature() string {
	names := make([]string, len(b.Args))
	for i, a := range b.Args {
		names[i] = a.Name
	}
	return b.Name + "(" + strings.Join(names, ", ") + ")"
}

// checkBuiltin holds a builtin literal to its table entry: the arity, every required
// argument bound by a positive body atom (never the anonymous variable), and a
// constant argument the predicate could never accept refused with the reason.
func checkBuiltin(a Atom, bound map[string]bool) error {
	b, _ := builtinNamed(a.Name)
	if len(a.Args) != len(b.Args) {
		return fmt.Errorf("%s takes %d arguments, %s, but is written with %d", b.Name, len(b.Args), b.signature(), len(a.Args))
	}
	consts := make([]*Const, len(a.Args))
	for i, t := range a.Args {
		spec := b.Args[i]
		switch {
		case !t.IsVar():
			c := t.Const
			consts[i] = &c
		case spec.Bound && (t.Var == "_" || !bound[t.Var]):
			return fmt.Errorf("%s: argument %s is %s, which no positive body atom binds; %s needs %s bound before it is read",
				b.Name, spec.Name, t.Var, b.Name, spec.Name)
		}
	}
	if b.check != nil {
		if err := b.check(consts); err != nil {
			return fmt.Errorf("%s: %w", b.Name, err)
		}
	}
	return nil
}

// builtinHolds evaluates a builtin literal under env. Every required argument is
// bound: the plan check saw to that.
func builtinHolds(l Literal, env map[string]Const) bool {
	b, _ := builtinNamed(l.Atom.Name)
	args := make([]Const, len(l.Atom.Args))
	for i, t := range l.Atom.Args {
		args[i] = value(t, env)
	}
	return b.holds(args) != l.Negated
}

// factTime reads a whole fact-time lexeme, the fixed-width string clock.Format writes.
// A shorter or looser spelling is not one: it sorts against the record wrongly, and
// the same rule holds instant/1.
func factTime(c Const) (time.Time, bool) {
	if c.kind != KindString {
		return time.Time{}, false
	}
	t, err := clock.Parse(c.s)
	if err != nil || clock.Format(t) != c.s {
		return time.Time{}, false
	}
	return t, true
}

func checkWithin(consts []*Const) error {
	example := "2026-06-15T00:00:00.000000000Z"
	for i, name := range []string{"A", "B"} {
		c := consts[i]
		if c == nil {
			continue
		}
		if _, ok := factTime(*c); !ok {
			return fmt.Errorf("argument %s, %s, is not a fact-time; write the whole instant, as %s (layout %s)", name, c.Text(), example, clock.Layout)
		}
	}
	if d := consts[2]; d != nil {
		if d.kind != KindNumber {
			return fmt.Errorf("argument Days, %s, is not a number of days", d.Text())
		}
		if d.num.sign() < 0 {
			return fmt.Errorf("argument Days, %s, is negative; a distance in days is not", d.Text())
		}
	}
	return nil
}

func holdsWithin(args []Const) bool {
	a, okA := factTime(args[0])
	b, okB := factTime(args[1])
	days := args[2]
	if !okA || !okB || days.kind != KindNumber || days.num.sign() < 0 {
		return false
	}
	return coversNanos(days.num, nanosBetween(a, b))
}

// nanosBetween is |a - b| in nanoseconds, exact over the whole range of times.
func nanosBetween(a, b time.Time) *big.Int {
	ns := new(big.Int).Sub(big.NewInt(a.Unix()), big.NewInt(b.Unix()))
	ns.Mul(ns, big.NewInt(1_000_000_000))
	ns.Add(ns, big.NewInt(int64(a.Nanosecond()-b.Nanosecond())))
	return ns.Abs(ns)
}

// nanosPerDayOverTen11 is a day in nanoseconds, 86400 * 10^9, as 864 * 10^11: the
// factor that lets a decimal count of days be scaled to nanoseconds by a power of ten.
const nanosPerDayOverTen11 = 864

// coversNanos reports whether n days, a number that is not negative, last at least
// ns nanoseconds. It is exact: the days are digits * 10^exp, so the comparison is
// ns <= digits * 864 * 10^(exp+11), done in integers with a power of ten only as
// large as the comparison can need.
func coversNanos(n number, ns *big.Int) bool {
	if n.digits == "0" {
		return ns.Sign() == 0
	}
	d, _ := new(big.Int).SetString(n.digits, 10)
	d.Mul(d, big.NewInt(nanosPerDayOverTen11))
	scale := n.exp + 11
	if scale >= 0 {
		// d >= 864, so past 10^18 the right side, over 8.6e20, exceeds any distance
		// between two instants the clock can write (years 0001 to 9999, under 3e20
		// nanoseconds).
		if scale > 18 {
			return true
		}
		d.Mul(d, pow10(scale))
		return ns.Cmp(d) <= 0
	}
	// ns * 10^-scale <= d. If ns is not zero, the left side has more digits than d
	// once the power is as long as d is.
	if ns.Sign() == 0 {
		return true
	}
	if -scale >= len(d.String()) {
		return false
	}
	return new(big.Int).Mul(ns, pow10(-scale)).Cmp(d) <= 0
}

func pow10(n int) *big.Int {
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}
