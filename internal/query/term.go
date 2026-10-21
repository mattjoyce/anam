package query

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Kind is the type of a constant. A query sees the three scalar kinds JSON has, and
// nothing else: a container at a pointer is not a value, and the leaves beneath it
// are.
type Kind uint8

// The kinds, in the order constants of different kinds sort.
const (
	KindString Kind = iota
	KindNumber
	KindBool
)

// Const is one constant: a string, a number or a boolean. Numbers are float64, as
// JSON readers commonly take them, and keep one canonical lexeme so that 1 and 1.0
// are the same constant and hash the same.
type Const struct {
	kind Kind
	s    string
	n    float64
}

// String makes a string constant.
func String(s string) Const { return Const{kind: KindString, s: s} }

// Bool makes a boolean constant.
func Bool(b bool) Const { return Const{kind: KindBool, s: strconv.FormatBool(b)} }

// Number makes a number constant from a JSON lexeme.
func Number(lexeme string) (Const, error) {
	n, err := strconv.ParseFloat(lexeme, 64)
	if err != nil {
		return Const{}, fmt.Errorf("%w: %q is not a number", ErrSyntax, lexeme)
	}
	return Const{kind: KindNumber, s: strconv.FormatFloat(n, 'g', -1, 64), n: n}, nil
}

// Kind reports the constant's kind.
func (c Const) Kind() Kind { return c.kind }

// Text is the constant as text: the string itself, the number's canonical lexeme,
// or true/false.
func (c Const) Text() string { return c.s }

// MarshalJSON writes the constant as the JSON value it came from.
func (c Const) MarshalJSON() ([]byte, error) {
	if c.kind == KindString {
		return json.Marshal(c.s)
	}
	return []byte(c.s), nil
}

// UnmarshalJSON reads a constant back from the JSON MarshalJSON wrote, so an answer
// decoded on the client holds the constants the server's did.
func (c *Const) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	switch t := v.(type) {
	case string:
		*c = String(t)
	case bool:
		*c = Bool(t)
	case json.Number:
		n, err := Number(t.String())
		if err != nil {
			return err
		}
		*c = n
	default:
		return fmt.Errorf("%w: a constant is a string, a number or a boolean, not %s", ErrSyntax, b)
	}
	return nil
}

// key is the constant's identity for sets and joins. The kind prefix keeps the
// string "1" apart from the number 1.
func (c Const) key() string {
	return string(rune('0'+c.kind)) + c.s
}

// compare orders two constants of one kind: strings lexically (fact-times are
// fixed-width, so that is chronological), numbers numerically, false before true.
// ok is false when the kinds differ: they are unequal, and not ordered.
func compare(a, b Const) (cmp int, ok bool) {
	if a.kind != b.kind {
		return 0, false
	}
	switch a.kind {
	case KindNumber:
		switch {
		case a.n < b.n:
			return -1, true
		case a.n > b.n:
			return 1, true
		}
		return 0, true
	default:
		return strings.Compare(a.s, b.s), true
	}
}

// less is the total order over constants an answer is sorted by: by kind, then by
// the kind's own order.
func less(a, b Const) bool {
	if a.kind != b.kind {
		return a.kind < b.kind
	}
	cmp, _ := compare(a, b)
	return cmp < 0
}

// Tuple is one row of a relation.
type Tuple []Const

func (t Tuple) key() string {
	var b strings.Builder
	for _, c := range t {
		b.WriteString(c.key())
		b.WriteByte(0)
	}
	return b.String()
}

// tupleLess orders tuples positionally.
func tupleLess(a, b Tuple) bool {
	for i := range a {
		if i >= len(b) {
			return false
		}
		if less(a[i], b[i]) {
			return true
		}
		if less(b[i], a[i]) {
			return false
		}
	}
	return len(a) < len(b)
}
