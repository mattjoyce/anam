package query

import (
	"fmt"
	"strconv"
	"strings"
)

// number is an exact decimal: the significant digits of a value and the power of
// ten they are scaled by. It keeps what the record holds. 1792269840000 stays
// 1792269840000, where a float64 rounded it to 1.79226984e+12 and made it the
// same number as its neighbour. It compares exactly, so two numbers a float64
// cannot tell apart stay two numbers.
//
// The form is canonical: the digits carry no leading or trailing zero and the
// exponent carries the scale. One value, one form: 1 and 1.0 are one number and
// hash the same, as they did when the engine held numbers as float64.
type number struct {
	neg    bool
	digits string // the significant digits
	exp    int    // the value is sign * digits * 10^exp
}

// maxNumberDigits is the floor on how long a number's plain decimal form may be.
// A payload can hold a short exponent that expands to a gigabyte of digits
// ("1e999999999"), and the form is built before the answer's own size bound is
// applied; past the floor the text is the exact scientific form, which is short
// and just as exact. It is a floor, not a setting: any value of it is arbitrary,
// and no number a record holds comes near it.
const maxNumberDigits = 64

// maxExponent bounds the exponent a lexeme may write, either way. It is an invariant,
// not a setting: it keeps the arithmetic on the exponent (the digits' count added to
// it, the scientific form's place) inside an int, where an exponent near the int's own
// limit wrapped past the guards and panicked (card 224). Nine digits keeps every
// number a record can usefully hold, "1e999999999" included, and a value beyond it is
// refused as out of range before any arithmetic is done.
const maxExponent = 999_999_999

// parseNumber reads a JSON number lexeme into an exact decimal. The value is what
// the lexeme says; nothing is rounded away.
func parseNumber(lexeme string) (number, error) {
	notANumber := fmt.Errorf("%q is not a number", lexeme)
	i := 0
	neg := false
	if i < len(lexeme) && lexeme[i] == '-' {
		neg = true
		i++
	}
	intPart, next, ok := digits(lexeme, i)
	if !ok || intPart == "" {
		return number{}, notANumber
	}
	i = next
	frac := ""
	if i < len(lexeme) && lexeme[i] == '.' {
		frac, i, ok = digits(lexeme, i+1)
		if !ok || frac == "" {
			return number{}, notANumber
		}
	}
	exp := 0
	if i < len(lexeme) && (lexeme[i] == 'e' || lexeme[i] == 'E') {
		i++
		sign := 1
		if i < len(lexeme) && (lexeme[i] == '+' || lexeme[i] == '-') {
			if lexeme[i] == '-' {
				sign = -1
			}
			i++
		}
		es, next, ok := digits(lexeme, i)
		if !ok || es == "" {
			return number{}, notANumber
		}
		i = next
		e, err := strconv.Atoi(es)
		if err != nil || e > maxExponent {
			return number{}, fmt.Errorf("%q has an exponent out of range", lexeme)
		}
		exp = sign * e
	}
	if i != len(lexeme) {
		return number{}, notANumber
	}

	d := intPart + frac
	exp -= len(frac)
	d = strings.TrimLeft(d, "0")
	if d == "" {
		return number{digits: "0", exp: 0}, nil // the value is zero
	}
	trailing := len(d) - len(strings.TrimRight(d, "0"))
	d = strings.TrimRight(d, "0")
	return number{neg: neg, digits: d, exp: exp + trailing}, nil
}

// digits reads the run of decimal digits at i, and where it ends.
func digits(s string, i int) (string, int, bool) {
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[start:i], i, i > start
}

// text is the number as an answer writes it: the exact decimal value, an integer
// in full, as the record holds it.
func (n number) text() string {
	if n.digits == "0" {
		return "0"
	}
	s, ok := n.plain()
	if !ok {
		s = n.scientific()
	}
	if n.neg {
		return "-" + s
	}
	return s
}

// plain is the decimal expansion of the value, and whether it is short enough to
// print.
func (n number) plain() (string, bool) {
	if n.exp >= 0 {
		if len(n.digits)+n.exp > maxNumberDigits {
			return "", false
		}
		return n.digits + strings.Repeat("0", n.exp), true
	}
	place := len(n.digits) + n.exp
	switch {
	case place > 0:
		if len(n.digits)+1 > maxNumberDigits {
			return "", false
		}
		return n.digits[:place] + "." + n.digits[place:], true
	case place == 0:
		if len(n.digits)+2 > maxNumberDigits {
			return "", false
		}
		return "0." + n.digits, true
	default:
		if 2-place+len(n.digits) > maxNumberDigits {
			return "", false
		}
		return "0." + strings.Repeat("0", -place) + n.digits, true
	}
}

// scientific is the exact value in scientific form, for a value whose plain form
// is too long to print.
func (n number) scientific() string {
	mantissa := n.digits[:1]
	if len(n.digits) > 1 {
		mantissa += "." + n.digits[1:]
	}
	e := len(n.digits) - 1 + n.exp
	if e < 0 {
		return mantissa + "e-" + strconv.Itoa(-e)
	}
	return mantissa + "e+" + strconv.Itoa(e)
}

// key is the number's identity for sets and joins: the canonical form, so that 1
// and 1.0 are one number, and 100 and 1e2 are one number.
func (n number) key() string {
	if n.neg && n.digits != "0" {
		return "-" + n.digits + "e" + strconv.Itoa(n.exp)
	}
	return n.digits + "e" + strconv.Itoa(n.exp)
}

// cmp orders two numbers exactly: by sign, then by magnitude.
func (n number) cmp(o number) int {
	ns, os := n.sign(), o.sign()
	if ns != os {
		if ns < os {
			return -1
		}
		return 1
	}
	if ns == 0 {
		return 0
	}
	m := n.magnitude(o)
	if n.neg {
		return -m
	}
	return m
}

// sign is the number's sign: -1, 0 or 1.
func (n number) sign() int {
	if n.digits == "0" {
		return 0
	}
	if n.neg {
		return -1
	}
	return 1
}

// magnitude compares the absolute values of two numbers that are not zero.
func (n number) magnitude(o number) int {
	nPlace := len(n.digits) + n.exp
	oPlace := len(o.digits) + o.exp
	if nPlace != oPlace {
		if nPlace < oPlace {
			return -1
		}
		return 1
	}
	a, b := n.digits, o.digits
	if len(a) < len(b) {
		a += strings.Repeat("0", len(b)-len(a))
	}
	if len(b) < len(a) {
		b += strings.Repeat("0", len(a)-len(b))
	}
	return strings.Compare(a, b)
}
