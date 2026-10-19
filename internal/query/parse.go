package query

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// ErrSyntax is returned for text that is not a program.
var ErrSyntax = errors.New("query: syntax")

// Term is one argument of an atom or one side of a comparison: a variable, the
// anonymous variable, or a constant.
type Term struct {
	Var   string // "" for a constant; "_" is the anonymous variable
	Const Const
}

// IsVar reports whether the term is a variable, the anonymous one included.
func (t Term) IsVar() bool { return t.Var != "" }

// Atom is a relation applied to terms.
type Atom struct {
	Name string
	Args []Term
	Line int
}

// Comparison is a guard between two terms: =, !=, <, <=, > or >=.
type Comparison struct {
	Op          string
	Left, Right Term
	Line        int
}

// Literal is one body element: a positive atom, a negated atom, or a comparison.
type Literal struct {
	Atom    *Atom
	Negated bool
	Compare *Comparison
}

// Rule is head :- body. A fact is a rule with no body.
type Rule struct {
	Head Atom
	Body []Literal
	Line int
}

// Program is parsed text: rules, and the relations declared without rules
// (name/arity.), which a later program defines.
type Program struct {
	Rules    []Rule
	Declared map[string]int
	Text     string
}

// Parse reads a program. The syntax is Anam's own and small:
//
//	% a comment runs to the end of the line
//	name/arity.                                  declares a relation another program defines
//	head(X, "c", 1).                             a fact
//	head(X, Y) :- rel(X, Z), not other(Z), Y > 3.  a rule
//
// A relation name is lower-case; a variable starts upper-case; _ is anonymous.
// Constants are double-quoted strings, numbers, true and false.
func Parse(text string) (Program, error) {
	lx := &lexer{src: text, line: 1}
	toks, err := lx.all()
	if err != nil {
		return Program{}, err
	}
	p := &parser{toks: toks, prog: Program{Declared: map[string]int{}, Text: text}}
	for !p.at(tokEOF) {
		if err := p.statement(); err != nil {
			return Program{}, err
		}
	}
	return p.prog, nil
}

type tokKind uint8

const (
	tokEOF tokKind = iota
	tokName
	tokVar
	tokString
	tokNumber
	tokPunct // ( ) , . :- / and the comparison operators
)

type token struct {
	kind tokKind
	text string
	line int
}

type lexer struct {
	src  string
	pos  int
	line int
}

func (lx *lexer) all() ([]token, error) {
	var toks []token
	for {
		t, err := lx.next()
		if err != nil {
			return nil, err
		}
		toks = append(toks, t)
		if t.kind == tokEOF {
			return toks, nil
		}
	}
}

func (lx *lexer) next() (token, error) {
	lx.skipSpace()
	if lx.pos >= len(lx.src) {
		return token{kind: tokEOF, line: lx.line}, nil
	}
	start, line := lx.pos, lx.line
	r := rune(lx.src[lx.pos])
	switch {
	case r == '"':
		return lx.str(line)
	case unicode.IsLower(r):
		return token{tokName, lx.word(), line}, nil
	case unicode.IsUpper(r) || r == '_':
		return token{tokVar, lx.word(), line}, nil
	case unicode.IsDigit(r) || r == '-':
		lx.number()
		return token{tokNumber, lx.src[start:lx.pos], line}, nil
	}
	for _, p := range []string{":-", "!=", "<=", ">=", "=", "<", ">", "(", ")", ",", ".", "/"} {
		if strings.HasPrefix(lx.src[lx.pos:], p) {
			lx.pos += len(p)
			return token{tokPunct, p, line}, nil
		}
	}
	return token{}, fmt.Errorf("%w: line %d: unexpected %q", ErrSyntax, line, r)
}

func (lx *lexer) skipSpace() {
	for lx.pos < len(lx.src) {
		switch lx.src[lx.pos] {
		case '\n':
			lx.line++
			lx.pos++
		case ' ', '\t', '\r':
			lx.pos++
		case '%':
			for lx.pos < len(lx.src) && lx.src[lx.pos] != '\n' {
				lx.pos++
			}
		default:
			return
		}
	}
}

// number scans a JSON number: an optional sign, digits, a fraction only when a digit
// follows the point (so the period that ends a statement is left alone), and an
// exponent.
func (lx *lexer) number() {
	digits := func() {
		for lx.pos < len(lx.src) && unicode.IsDigit(rune(lx.src[lx.pos])) {
			lx.pos++
		}
	}
	if lx.src[lx.pos] == '-' {
		lx.pos++
	}
	digits()
	if lx.pos+1 < len(lx.src) && lx.src[lx.pos] == '.' && unicode.IsDigit(rune(lx.src[lx.pos+1])) {
		lx.pos++
		digits()
	}
	if lx.pos < len(lx.src) && (lx.src[lx.pos] == 'e' || lx.src[lx.pos] == 'E') {
		lx.pos++
		if lx.pos < len(lx.src) && (lx.src[lx.pos] == '+' || lx.src[lx.pos] == '-') {
			lx.pos++
		}
		digits()
	}
}

func (lx *lexer) word() string {
	start := lx.pos
	for lx.pos < len(lx.src) {
		c := rune(lx.src[lx.pos])
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) && c != '_' {
			break
		}
		lx.pos++
	}
	return lx.src[start:lx.pos]
}

// str reads a double-quoted string with Go/JSON escapes; the quoted text is handed to
// strconv.Unquote rather than a second escape table.
func (lx *lexer) str(line int) (token, error) {
	start := lx.pos
	lx.pos++
	for lx.pos < len(lx.src) {
		switch lx.src[lx.pos] {
		case '\\':
			lx.pos += 2
			continue
		case '"':
			lx.pos++
			s, err := strconv.Unquote(lx.src[start:lx.pos])
			if err != nil {
				return token{}, fmt.Errorf("%w: line %d: bad string %s", ErrSyntax, line, lx.src[start:lx.pos])
			}
			return token{tokString, s, line}, nil
		case '\n':
			return token{}, fmt.Errorf("%w: line %d: string runs past the end of the line", ErrSyntax, line)
		}
		lx.pos++
	}
	return token{}, fmt.Errorf("%w: line %d: unterminated string", ErrSyntax, line)
}

// comparisonOps are the guards a body may hold.
var comparisonOps = map[string]bool{"=": true, "!=": true, "<": true, "<=": true, ">": true, ">=": true}

type parser struct {
	toks []token
	i    int
	prog Program
}

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) at(k tokKind, text ...string) bool {
	t := p.peek()
	return t.kind == k && (len(text) == 0 || t.text == text[0])
}

func (p *parser) take() token {
	t := p.toks[p.i]
	if t.kind != tokEOF {
		p.i++
	}
	return t
}

func (p *parser) expect(text string) error {
	if !p.at(tokPunct, text) {
		return p.fail("expected %q", text)
	}
	p.take()
	return nil
}

func (p *parser) fail(format string, args ...any) error {
	t := p.peek()
	got := t.text
	if t.kind == tokEOF {
		got = "end of program"
	}
	return fmt.Errorf("%w: line %d: %s, got %q", ErrSyntax, t.line, fmt.Sprintf(format, args...), got)
}

// statement is a declaration (name/arity.), a fact, or a rule.
func (p *parser) statement() error {
	if !p.at(tokName) {
		return p.fail("expected a relation name")
	}
	line := p.peek().line
	if p.toks[p.i+1].kind == tokPunct && p.toks[p.i+1].text == "/" {
		name := p.take().text
		p.take()
		if !p.at(tokNumber) {
			return p.fail("expected an arity after %s/", name)
		}
		arity, err := strconv.Atoi(p.take().text)
		if err != nil || arity < 1 {
			return fmt.Errorf("%w: line %d: %s/%v is not a relation: arity must be a positive integer", ErrSyntax, line, name, arity)
		}
		p.prog.Declared[name] = arity
		return p.expect(".")
	}
	head, err := p.atom()
	if err != nil {
		return err
	}
	r := Rule{Head: head, Line: line}
	if p.at(tokPunct, ":-") {
		p.take()
		for {
			lit, err := p.literal()
			if err != nil {
				return err
			}
			r.Body = append(r.Body, lit)
			if !p.at(tokPunct, ",") {
				break
			}
			p.take()
		}
	}
	if err := p.expect("."); err != nil {
		return err
	}
	p.prog.Rules = append(p.prog.Rules, r)
	return nil
}

func (p *parser) literal() (Literal, error) {
	if p.at(tokName, "not") {
		p.take()
		a, err := p.atom()
		if err != nil {
			return Literal{}, err
		}
		return Literal{Atom: &a, Negated: true}, nil
	}
	if p.at(tokName) {
		a, err := p.atom()
		if err != nil {
			return Literal{}, err
		}
		return Literal{Atom: &a}, nil
	}
	line := p.peek().line
	left, err := p.term()
	if err != nil {
		return Literal{}, err
	}
	op := p.peek()
	if op.kind != tokPunct || !comparisonOps[op.text] {
		return Literal{}, p.fail("expected a comparison operator")
	}
	p.take()
	right, err := p.term()
	if err != nil {
		return Literal{}, err
	}
	return Literal{Compare: &Comparison{Op: op.text, Left: left, Right: right, Line: line}}, nil
}

func (p *parser) atom() (Atom, error) {
	if !p.at(tokName) {
		return Atom{}, p.fail("expected a relation name")
	}
	t := p.take()
	a := Atom{Name: t.text, Line: t.line}
	if err := p.expect("("); err != nil {
		return Atom{}, err
	}
	for {
		arg, err := p.term()
		if err != nil {
			return Atom{}, err
		}
		a.Args = append(a.Args, arg)
		if p.at(tokPunct, ")") {
			p.take()
			return a, nil
		}
		if err := p.expect(","); err != nil {
			return Atom{}, err
		}
	}
}

func (p *parser) term() (Term, error) {
	t := p.peek()
	switch t.kind {
	case tokVar:
		p.take()
		return Term{Var: t.text}, nil
	case tokString:
		p.take()
		return Term{Const: String(t.text)}, nil
	case tokNumber:
		p.take()
		c, err := Number(t.text)
		if err != nil {
			return Term{}, fmt.Errorf("line %d: %w", t.line, err)
		}
		return Term{Const: c}, nil
	case tokName:
		if t.text == "true" || t.text == "false" {
			p.take()
			return Term{Const: Bool(t.text == "true")}, nil
		}
	}
	return Term{}, p.fail("expected a variable or a constant")
}
