package query

import (
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/mattjoyce/tarski"

	"github.com/mattjoyce/anam/internal/clock"
)

// A builtin is a predicate answered in Go, because the language has no arithmetic to
// write it in (card 212). It reads like an atom in a body, but it names no relation:
// it is a guard, as a comparison is, true or false of constants already bound. Tarski
// runs it (tarski.Builtin); this table is where Anam's are declared, and where the
// next one goes (card 213's month and day functions).

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

// tarskiBuiltins is the table as tarski declares it.
func tarskiBuiltins() map[string]tarski.Builtin {
	out := make(map[string]tarski.Builtin, len(builtins))
	for _, b := range builtins {
		args := make([]tarski.BuiltinArg, len(b.Args))
		for i, a := range b.Args {
			args[i] = tarski.BuiltinArg{Name: a.Name, Doc: a.Doc}
		}
		out[b.Name] = tarski.Builtin{Args: args, Doc: b.Doc, Check: b.check, Holds: b.holds}
	}
	return out
}

// factTime reads a whole fact-time lexeme, the fixed-width string clock.Format writes.
// A shorter or looser spelling is not one: it sorts against the record wrongly, and
// the same rule holds instant/1.
func factTime(c Const) (time.Time, bool) {
	if c.Kind() != KindString {
		return time.Time{}, false
	}
	t, err := clock.Parse(c.Text())
	if err != nil || clock.Format(t) != c.Text() {
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
		if d.Kind() != KindNumber {
			return fmt.Errorf("argument Days, %s, is not a number of days", d.Text())
		}
		if n, ok := readDecimal(d.Text()); ok && n.negative() {
			return fmt.Errorf("argument Days, %s, is negative; a distance in days is not", d.Text())
		}
	}
	return nil
}

func holdsWithin(args []Const) bool {
	a, okA := factTime(args[0])
	b, okB := factTime(args[1])
	if !okA || !okB || args[2].Kind() != KindNumber {
		return false
	}
	days, ok := readDecimal(args[2].Text())
	if !ok || days.negative() {
		return false
	}
	return coversNanos(days, nanosBetween(a, b))
}

// decimal is a number's value as digits and a power of ten: the value is
// digits * 10^exp, with no trailing zero in digits, and neg its sign. It is read from
// a constant's canonical text, which is exact.
type decimal struct {
	neg    bool
	digits string
	exp    int
}

func (d decimal) negative() bool { return d.neg && d.digits != "0" }

// readDecimal reads a number constant's canonical text: an optional minus, digits
// with an optional fraction, and an optional exponent. ok is false for any other text.
func readDecimal(text string) (decimal, bool) {
	var d decimal
	s := text
	if strings.HasPrefix(s, "-") {
		d.neg, s = true, s[1:]
	}
	mant, expText, _ := strings.Cut(strings.ToLower(s), "e")
	whole, frac, _ := strings.Cut(mant, ".")
	digits := strings.TrimLeft(whole+frac, "0")
	if whole == "" || strings.Trim(whole+frac, "0123456789") != "" {
		return decimal{}, false
	}
	exp := -len(frac)
	if expText != "" {
		var e big.Int
		if _, ok := e.SetString(strings.TrimPrefix(expText, "+"), 10); !ok || !e.IsInt64() || e.Int64() > 1<<31 || e.Int64() < -(1<<31) {
			return decimal{}, false
		}
		exp += int(e.Int64())
	}
	if digits == "" {
		return decimal{digits: "0"}, true
	}
	trimmed := strings.TrimRight(digits, "0")
	d.digits, d.exp = trimmed, exp+len(digits)-len(trimmed)
	return d, true
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
func coversNanos(n decimal, ns *big.Int) bool {
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
