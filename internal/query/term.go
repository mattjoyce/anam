package query

import "github.com/mattjoyce/tarski"

// The language's vocabulary is the tarski module's (card 230): the package names its
// types here so that callers keep importing internal/query and errors.Is holds across
// the boundary.

// Kind is the type of a constant: a string, a number or a boolean.
type Kind = tarski.Kind

// The kinds, in the order constants of different kinds sort.
const (
	KindString = tarski.KindString
	KindNumber = tarski.KindNumber
	KindBool   = tarski.KindBool
)

// The program and its parts, and the answer.
type (
	// Const is one constant: a string, a number as an exact decimal, or a boolean.
	Const = tarski.Const
	// Tuple is one row of a relation.
	Tuple = tarski.Tuple
	// Program is the parsed rules, facts and declarations of a text.
	Program = tarski.Program
	// Rule is head :- body.
	Rule = tarski.Rule
	// Atom is a relation applied to terms.
	Atom = tarski.Atom
	// Term is a variable or a constant.
	Term = tarski.Term
	// Literal is one body element: an atom, its negation or a comparison.
	Literal = tarski.Literal
	// Comparison is a comparison of two terms.
	Comparison = tarski.Comparison
	// Answer is one relation of a result: sorted tuples, one serialisation, a hash.
	Answer = tarski.Answer
	// TooLarge is the refusal of a derivation over the byte bound.
	TooLarge = tarski.TooLarge
	// Plan is a validated, stratified program, ready to evaluate.
	Plan = tarski.Plan
	// Base is the extensional database: the base relations over one served value.
	Base = tarski.Base
	// Result is what an evaluation derived.
	Result = tarski.Result
)

// The refusals, the module's own sentinels.
var (
	// ErrInvalid is returned for a program that parses but cannot run: an unknown
	// relation, a wrong arity, an unsafe variable, a negation that cannot be
	// stratified, a rule over a base relation, or a pointer no Definition declares.
	// Each message says what to change.
	ErrInvalid = tarski.ErrInvalid
	// ErrSyntax is returned for a text that is not a program.
	ErrSyntax = tarski.ErrSyntax
	// ErrTooLarge marks a derivation refused for its size; asking again unchanged
	// will fail again.
	ErrTooLarge = tarski.ErrTooLarge
)

// String, Bool and Number make constants.
var (
	String = tarski.String
	Bool   = tarski.Bool
	Number = tarski.Number
)
