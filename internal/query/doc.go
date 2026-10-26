// Package query is the third read (ARCHITECTURE.md §9, card 173): a Datalog query
// over a served value. The engine is the tarski module (github.com/mattjoyce/tarski
// v0.3.0, card 230): it parses, validates and evaluates. This package supplies what is
// Anam's, the base relations over a served value, the checks on instant/1 and on a
// constant attr/3 pointer, within/3, the bundled rules, and the refusals that keep the
// language what it was: no aggregates, no prefix, suffix or contains, and the module's
// own refusals of a number with a leading zero, true or false as a relation name, and
// text that is not UTF-8. Its types are tarski's, named here as aliases. Anam is Datomic-shaped, and a query is a pure
// function of a database value; fold.Value at a watermark is that value, after a
// projection has decided which whole facets the query may see.
//
// The language is small (Parse): rules with named derived relations,
// stratified negation, comparisons, facts, and a declaration for a relation one
// program leaves to another. Compile validates a program before anything is read,
// unsafe variables, unknown relations and unstratifiable negation each refused with a
// message that says what to change, and Evaluate runs it semi-naive to a fixpoint. It
// always terminates: no function symbols, so the derived relations are finite.
//
// The base relations (NewBase) are the envelope, entry/7, every scalar leaf of each
// served payload by its JSON pointer, attr/3, and the Definitions' word on which types
// close a series, closes/1. The payload is read from the served copy, never from the
// store (§8). The bundled rule set (Bundled) is Anam's reading of its own time
// semantics, current/2 and displaced/2, which a test holds equal to fold.Snapshot, so
// there is one account of liveness while there are two implementations of it.
//
// Everything here is read-side and disposable, outside the error kernel: it takes no
// ledger, moves no watermark and writes nothing. An answer (Result.Relation) is a
// value that hashes. The daemon route, the CLI, the receipt, introspection and the
// size bound are the card's later slices.
package query
