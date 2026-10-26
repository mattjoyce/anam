package query_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/mattjoyce/tarski"

	"github.com/mattjoyce/anam/internal/fold"
	"github.com/mattjoyce/anam/internal/query"
)

// Card 230, slice 1: the package's own engine and the tarski module run side by side
// over the same programs and the same served values, and must answer with the same
// bytes and the same hash, or refuse in the same class. A difference is a finding. It
// stays in knownDiffs, with the reason, so the test pins it and fails if it goes away
// unnoticed; nothing here bends either engine to match.

// knownDiffs maps a case id, fixture/name, to what differs and why.
var knownDiffs = map[string]string{
	// Numbers: tarski reads a number as JSON spells it, so a leading zero is no number.
	// The package's own lexer took 01 as 1. Saved selections and held rule sets that
	// hold one are what the pre-deploy re-parse on unraid looks for.
	"refusals/leading-zero-number":     leadingZero,
	"refusals/leading-zero-comparison": leadingZero,
	"numbers/fact-01":                  leadingZero,
	"numbers/const-01":                 leadingZero,
	"numbers/fact--01":                 leadingZero,
	"numbers/const--01":                leadingZero,
	"numbers/fact-00":                  leadingZero,
	"numbers/const-00":                 leadingZero,

	// Text: tarski refuses a string that is not valid UTF-8, where the package's own
	// engine took the bytes.
	"refusals/parse-invalid-utf8-string": "tarski's lexer refuses invalid UTF-8 inside a string; the package's own accepted it",

	// Names: tarski reserves prefix, suffix and contains as engine builtins, and Anam
	// refuses them by decision (card 230), so a user relation of that name, which the
	// package's own engine accepted, is now refused.
	"refusals/user-relation-prefix-head":     reservedName,
	"refusals/user-relation-prefix-fact":     reservedName,
	"refusals/user-relation-suffix-declared": reservedName,
	"refusals/user-relation-contains-query":  reservedName,
	"small/relation-named-true":              "tarski's lexer reads true and false as constants everywhere, so true(X) is no relation; the package's own parser accepted a relation named true",
}

const (
	leadingZero  = "tarski reads a number as JSON spells it; a leading zero is not a number there and the package's own lexer took it"
	reservedName = "prefix, suffix and contains are reserved names under tarski and refused by Anam's decision; a user relation of that name was valid before"
)

type diffFixture struct {
	name     string
	v        fold.Value
	versions map[string]int
	defs     query.Definitions
}

type diffCase struct{ name, text, relation string }

// heldTexts is every rule set text the binary holds, by hash.
func heldTexts(t *testing.T) map[string]string {
	t.Helper()
	texts, err := query.HeldTexts()
	if err != nil {
		t.Fatal(err)
	}
	return texts
}

// errClass names the family of a refusal, so the two engines' sentinels compare.
func errClass(err error, invalid, syntax, large error) string {
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, large):
		return "too-large"
	case errors.Is(err, invalid):
		return "invalid"
	case errors.Is(err, syntax):
		return "syntax"
	case errors.Is(err, query.ErrBase):
		return "base"
	}
	return "other"
}

// normalise drops the engine prefix a message carries, so wording can be compared.
func normalise(err error) string {
	s := err.Error()
	for _, p := range []string{"query: ", "tarski: "} {
		s = strings.ReplaceAll(s, p, "")
	}
	return s
}

// report records a difference: a known one is logged, an unknown one fails the test.
func report(t *testing.T, id, diff string) {
	t.Helper()
	if why, ok := knownDiffs[id]; ok {
		t.Logf("known difference %s: %s (%s)", id, diff, why)
		return
	}
	t.Errorf("DIFFERENCE %s: %s", id, diff)
}

// settled is called when a case agrees: a known difference that has gone is a stale
// pin.
func settled(t *testing.T, id string) {
	t.Helper()
	if why, ok := knownDiffs[id]; ok {
		t.Errorf("%s is listed as a known difference (%s) but the engines now agree", id, why)
	}
}

// differ runs one program under both engines, with the rule set named by hash, and
// holds them to the same answer or the same class of refusal.
func differ(t *testing.T, fx diffFixture, rulesHash string, c diffCase, maxBytes int) {
	t.Helper()
	id := fx.name + "/" + c.name
	if maxBytes != 0 {
		id = fmt.Sprintf("%s@%d", id, maxBytes)
	}
	rs, ok, err := query.HeldRuleSet(rulesHash)
	if err != nil || !ok {
		t.Fatalf("rule set %s: %v, held %v", rulesHash, err, ok)
	}
	ctx := context.Background()
	own, ownErr := query.RunWith(ctx, rs, fx.v, fx.versions, fx.defs, c.text, c.relation, maxBytes)
	th, thErr := query.RunTarskiWith(ctx, heldTexts(t)[rulesHash], fx.v, fx.versions, fx.defs, c.text, c.relation, maxBytes)

	oc := errClass(ownErr, query.ErrInvalid, query.ErrSyntax, query.ErrTooLarge)
	tc := errClass(thErr, tarski.ErrInvalid, tarski.ErrSyntax, tarski.ErrTooLarge)
	switch {
	case oc == "ok" && tc == "ok":
		switch {
		case !bytes.Equal(own.Bytes(), th.Bytes()):
			report(t, id, fmt.Sprintf("answer bytes differ:\n  own    %s\n  tarski %s", clip(own.Bytes()), clip(th.Bytes())))
		case own.Hash() != th.Hash():
			report(t, id, fmt.Sprintf("hashes differ over equal bytes: %s vs %s", own.Hash(), th.Hash()))
		default:
			t.Logf("agree %s: %d tuples", id, own.Len())
			settled(t, id)
		}
	case oc == "ok":
		report(t, id, fmt.Sprintf("own answers (%d tuples), tarski refuses (%s): %v", own.Len(), tc, thErr))
	case tc == "ok":
		report(t, id, fmt.Sprintf("own refuses (%s): %v; tarski answers (%d tuples)", oc, ownErr, th.Len()))
	case oc != tc:
		report(t, id, fmt.Sprintf("both refuse, in different classes: own %s: %v; tarski %s: %v", oc, ownErr, tc, thErr))
	default:
		if oc == "too-large" {
			var o *query.TooLarge
			tl, _ := query.TarskiTooLarge(thErr)
			if errors.As(ownErr, &o) && tl != nil &&
				(o.Relation != tl.Relation || o.Tuples != tl.Tuples || o.Bytes != tl.Bytes || o.MaxBytes != tl.MaxBytes) {
				report(t, id, fmt.Sprintf("both refuse as too large, with different fields: own %+v; tarski %+v", *o, *tl))
				return
			}
		}
		if a, b := normalise(ownErr), normalise(thErr); a != b {
			t.Logf("message differs, %s:\n  own    %s\n  tarski %s", id, a, b)
		}
		t.Logf("agree %s: both refuse, %s", id, oc)
		settled(t, id)
	}
}

func clip(b []byte) string {
	if len(b) > 400 {
		return string(b[:400]) + "..."
	}
	return string(b)
}

func bundledHash(t *testing.T) string {
	t.Helper()
	rs, err := query.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	return rs.Hash
}

// runCases runs every case of a fixture under the bundled rules.
func runCases(t *testing.T, fx diffFixture, cases []diffCase) {
	t.Helper()
	hash := bundledHash(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			differ(t, fx, hash, c, 0)
		})
	}
}

func emptyFixture(t *testing.T) diffFixture {
	t.Helper()
	v, versions := value(t)
	return diffFixture{name: "empty", v: v, versions: versions, defs: registry(t)}
}

func regimenDiffFixture(t *testing.T) diffFixture {
	t.Helper()
	h := newHarness(t)
	h.seedRegimen(t)
	h.admit(t, "health-db", "health.weight", "2026-06-15T06:00:00Z", `{"measured_at": "2026-06-15T06:00:00Z", "kg": 81}`)
	v, versions, _ := h.served(t)
	return diffFixture{name: "regimen", v: v, versions: versions, defs: h.defs}
}

// Every worked question and refused program in docs/QUERY.md.
func TestDifferential_DocsWorkedQuestions(t *testing.T) {
	t.Parallel()
	h, v, versions := docsLedger(t)
	fx := diffFixture{name: "docs", v: v, versions: versions, defs: h.defs}
	var cases []diffCase
	for _, b := range docBlocks(t) {
		if b.lang != "datalog" {
			continue
		}
		name := b.attrs["question"]
		if name == "" {
			name = "refused-" + b.attrs["refused"]
		}
		if name == "refused-" {
			continue
		}
		cases = append(cases, diffCase{name, b.body, b.attrs["relation"]})
	}
	if len(cases) < 8 {
		t.Fatalf("%d doc programs, want at least 8", len(cases))
	}
	runCases(t, fx, cases)
}

// The ledger fixture's questions, the equality program, in_force, the edges, the
// lazy attr programs and the container pointers.
func TestDifferential_LedgerQuestions(t *testing.T) {
	t.Parallel()
	fx := regimenDiffFixture(t)
	extra := ""
	for _, at := range []string{day("2025-12-31"), day("2026-05-01"), day("2026-07-15"), "2026-07-15T12:00:00.000000000Z", day("2027-01-01")} {
		extra += fmt.Sprintf("instant(%q).\n", at)
	}
	const bpInstants = `instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).`
	runCases(t, fx, []diffCase{
		{"equality-current", "instant(At) :- entry(_, _, At, _, _, _, _).\n" + extra, "current"},
		{"equality-displaced", "instant(At) :- entry(_, _, At, _, _, _, _).\n" + extra, "displaced"},
		{"equality-latest", "instant(At) :- entry(_, _, At, _, _, _, _).\n" + extra, "latest"},
		{"equality-closed", "instant(At) :- entry(_, _, At, _, _, _, _).\n" + extra, "closed"},
		{"equality-before", "", "before"},
		{"equality-later", "", "later"},
		{"in_force", bpInstants, "in_force"},
		{"in_force-current", bpInstants, "current"},
		{"in_force-needs-instants", `held(E) :- in_force(E, _).`, "held"},
		{"regimen-at-each-reading", `
			instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
			regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
			regimen(At, Drug) :- entry(_, "health.blood-pressure", At, _, _, _, _), current(E, At),
			                     entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", Drug).`, "regimen"},
		{"days-without-d3k2", `
			instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
			regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
			on_d3k2(At) :- current(E, At), entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", "vitamin d3 + k2").
			without(At) :- entry(_, "health.blood-pressure", At, _, _, _, _),
			               At >= "2026-06-01T00:00:00.000000000Z", At < "2026-09-01T00:00:00.000000000Z", not on_d3k2(At).`, "without"},
		{"high-with-regimen", `
			instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
			high(At, S, Drug) :- entry(R, "health.blood-pressure", At, _, _, _, _), attr(R, "/systolic", S), S > 140,
			                     current(E, At), entry(E, "health.medication.started", _, _, _, _, _), attr(E, "/drug", Drug).`, "high"},
		{"attr-all", `leaf(E, P, V) :- attr(E, P, V).`, "leaf"},
		{"attr-asked-for-itself", ``, "attr"},
		{"entry-asked-for-itself", ``, "entry"},
		{"closes-asked-for-itself", ``, "closes"},
		{"linked-no-edges", ``, "linked"},
		{"bad-bundled-relation", ``, "nothing"},
	})
}

// A fixture with edges, as the lazy and edge tests build it.
func TestDifferential_EdgesAndLazyAttr(t *testing.T) {
	t.Parallel()
	h, v, versions := lazyFixture(t)
	fx := diffFixture{name: "edges", v: v, versions: versions, defs: h.defs}
	runCases(t, fx, []diffCase{
		{"entry-only", `bp(E, At) :- entry(E, "health.blood-pressure", At, _, _, _, _).`, "bp"},
		{"closes", `c(T) :- closes(T).`, "c"},
		{"bundled-current-no-attr", `instant(At) :- entry(_, _, At, _, _, _, _). now(E) :- current(E, At).`, "now"},
		{"attr-directly", `high(E, V) :- attr(E, "/systolic", V), V > 130.`, "high"},
		{"attr-by-variable-pointer", `leaf(E, P, V) :- attr(E, P, V).`, "leaf"},
		{"attr-through-linked", `touched(E) :- linked(E, _).`, "touched"},
		{"attr-through-edge_to", `into(E, F) :- edge_to(E, F).`, "into"},
		{"attr-through-chain", `a(E) :- linked(E, _). b(E) :- a(E). c(E) :- entry(E, _, _, _, _, _, _), not b(E).`, "c"},
		{"attr-unreached", `unused(E) :- attr(E, "/systolic", _). bp(E) :- entry(E, "health.blood-pressure", _, _, _, _, _).`, "bp"},
		{"linked", ``, "linked"},
		{"edge_to", ``, "edge_to"},
		{"edge_from", ``, "edge_from"},
		{"reach", `reach(E, F) :- linked(E, F).
reach(E, G) :- reach(E, F), linked(X, F), linked(X, G), X != E, G != F.`, "reach"},
		{"negation-of-attr", `a(E) :- entry(E, _, _, _, _, _, _), not x(E). x(E) :- attr(E, _, _).`, "a"},
		{"recursive-over-linked", `r(E) :- r(E), linked(E, _). r(E) :- entry(E, _, _, _, _, _, _).`, "r"},
	})
}

// The evaluator tests' programs over small hand-built values.
func TestDifferential_EvaluatorPrograms(t *testing.T) {
	t.Parallel()
	v, versions := value(t,
		entry("a", "health.medication.started", day("2026-01-01"), "", `{}`),
		entry("b", "health.medication.started", day("2026-01-02"), "", `{}`),
		entry("c", "health.medication.started", day("2026-01-03"), "", `{}`),
		entry("d", "health.medication.started", day("2026-01-04"), "", `{}`),
		entry("r1", "health.blood-pressure", day("2026-02-01"), "", `{"measured_at": "x", "systolic": 118, "tags": ["am", "sitting"], "cuff": {"arm": "left"}}`),
		entry("r2", "health.blood-pressure", day("2026-02-02"), "", `{"measured_at": "x", "systolic": 146.0, "diastolic": null}`),
		entry("r3", "health.blood-pressure", day("2026-02-03"), "", `{"measured_at": "x", "systolic": 9, "ok": true, "name": "caf\u00e9 \"q\""}`),
	)
	fx := diffFixture{name: "small", v: v, versions: versions, defs: registry(t)}
	runCases(t, fx, []diffCase{
		{"recursion", `
			next(E, F) :- entry(E, "health.medication.started", T, _, _, _, _), entry(F, "health.medication.started", U, _, _, _, _), T < U, not between(E, F).
			between(E, F) :- entry(E, "health.medication.started", T, _, _, _, _), entry(F, "health.medication.started", U, _, _, _, _), entry(_, "health.medication.started", V, _, _, _, _), T < V, V < U.
			reaches(E, F) :- next(E, F).
			reaches(E, F) :- reaches(E, G), next(G, F).`, "reaches"},
		{"numeric-order", `high(E) :- attr(E, "/systolic", S), S > 140.`, "high"},
		{"not-lexical", `low(E) :- attr(E, "/systolic", S), S < 100.`, "low"},
		{"146.0-is-146", `exact(E) :- attr(E, "/systolic", 146).`, "exact"},
		{"nested-pointer", `left(E) :- attr(E, "/cuff/arm", "left").`, "left"},
		{"array-item", `tagged(E, X) :- attr(E, "/tags/1", X).`, "tagged"},
		{"null-is-absent", `dia(E) :- attr(E, "/diastolic", _).`, "dia"},
		{"string-vs-number", `odd(E) :- attr(E, "/systolic", S), S != "118".`, "odd"},
		{"unordered-kinds", `never(E) :- attr(E, "/systolic", S), S < "z".`, "never"},
		{"bool-vs-number", `b(E) :- attr(E, "/ok", V), V < 1.`, "b"},
		{"bool-order", `b(E) :- attr(E, "/ok", V), V > false.`, "b"},
		{"bool-equal", `b(E) :- attr(E, "/ok", true).`, "b"},
		{"string-escapes", `n(E) :- attr(E, "/name", "caf\u00e9 \"q\"").`, "n"},
		{"string-out", `n(E, V) :- attr(E, "/name", V).`, "n"},
		{"version-is-a-number", `v(E) :- entry(E, _, _, _, _, _, 1).`, "v"},
		{"version-string", `v(E) :- entry(E, _, _, _, _, _, "1").`, "v"},
		{"facts", `kind("bp"). kind("sleep"). k(X) :- kind(X), X != "sleep".`, "k"},
		{"ids", `ids(E, N) :- entry(E, _, _, _, _, _, N).`, "ids"},
		{"no-such-relation", `ids(E, N) :- entry(E, _, _, _, _, _, N).`, "nothing"},
		{"base-relation-as-answer", ``, "entry"},
		{"empty-answer", `none(E) :- entry(E, "nothing", _, _, _, _, _).`, "none"},
		{"declared-defined", `x/1. x(E) :- entry(E, _, _, _, _, _, _).`, "x"},
		{"declared-undefined", `x/1. y(E) :- x(E).`, "y"},
		{"declared-undefined-unreached", `x/1. y(E) :- entry(E, _, _, _, _, _, _).`, "y"},
		{"cross-product", `p(E, F) :- entry(E, _, _, _, _, _, _), entry(F, _, _, _, _, _, _), E != F.`, "p"},
		{"const-comparison", `p(E) :- entry(E, _, _, _, _, _, _), 1 < 2.`, "p"},
		{"neg-of-fact", `kind("a"). p(E) :- entry(E, _, _, _, _, _, _), not kind(E).`, "p"},
		{"neg-anonymous", `p(E) :- entry(E, _, _, _, _, _, _), not attr(E, _, _).`, "p"},
		{"head-const", `p("k", E) :- entry(E, "health.blood-pressure", _, _, _, _, _).`, "p"},
		{"repeated-variable", `p(E) :- entry(E, _, _, _, _, _, E).`, "p"},
		{"unicode-string", `p("héllo ☃"). q(X) :- p(X).`, "q"},
		{"empty-string", `p(""). q(X) :- p(X).`, "q"},
		{"underscore-variable-name", `p(_X) :- entry(_X, _, _, _, _, _, _).`, "p"},
		{"relation-named-count", `count(E) :- entry(E, _, _, _, _, _, _).`, "count"},
		{"relation-named-sum", `sum(E) :- entry(E, _, _, _, _, _, _).`, "sum"},
		{"relation-named-min", `min(E) :- entry(E, _, _, _, _, _, _).`, "min"},
		{"relation-named-avg", `avg(E) :- entry(E, _, _, _, _, _, _).`, "avg"},
		{"relation-named-true", `true(E) :- entry(E, _, _, _, _, _, _).`, "true"},
		{"relation-named-not", `not(E) :- entry(E, _, _, _, _, _, _).`, "not"},
		{"relation-named-within", `within(E) :- entry(E, _, _, _, _, _, _).`, "within"},
		{"relation-named-instant-fact", `instant("2026-01-01T00:00:00.000000000Z").`, "instant"},
		{"relation-named-before", `before(E) :- entry(E, _, _, _, _, _, _).`, "before"},
		{"mixed-arity-fact", `p(1). p(1, 2).`, "p"},
		{"long-identifier", strings.Repeat("a", 300) + `(X) :- entry(X, _, _, _, _, _, _).`, strings.Repeat("a", 300)},
		{"windows-newlines", "p(E) :-\r\n entry(E, _, _, _, _, _, _).\r\n", "p"},
		{"comment-in-string", `p("a % b"). q(X) :- p(X). % trailing`, "q"},
		{"dollar-in-text", `p("$x"). q(X) :- p(X).`, "q"},
		{"empty-program", ``, "current"},
	})
}

// Number constants in query text and in payloads, the precision cases and the
// exponent bounds.
func TestDifferential_Numbers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	lexemes := []string{"120", "81.4", "1792269840000", "9007199254740993", "9007199254740992", "1.0000000000000000001",
		"0.5", "-0.5", "1.0", "1e3", "-0", "0.0", "1E2", "1.5e-3", "100000000000000000000000000000000000000", "1e999999999"}
	for i, l := range lexemes {
		at := fmt.Sprintf("2026-06-%02dT07:00:00Z", i+1)
		h.admit(t, "bp-csv", "health.blood-pressure", at, fmt.Sprintf(`{"measured_at": %q, "systolic": %s}`, at, l))
	}
	v, versions, _ := h.served(t)
	fx := diffFixture{name: "numbers", v: v, versions: versions, defs: h.defs}
	const sys = `sys(E, S) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/systolic", S).`
	cases := []diffCase{
		{"attr-numbers", sys, "sys"},
		{"beyond-float64-match", sys + ` m(E) :- sys(E, S), S = 9007199254740993.`, "m"},
		{"float64-neighbour", sys + ` m(E) :- sys(E, S), S = 9007199254740992.`, "m"},
		{"one-vs-near-one", sys + ` m(E) :- sys(E, S), S = 1.`, "m"},
		{"ordering", sys + ` m(E) :- sys(E, S), S < 100.`, "m"},
		{"ordering-ge", sys + ` m(E) :- sys(E, S), S >= 1792269840000.`, "m"},
		{"neg-zero", sys + ` m(E) :- sys(E, S), S = 0.`, "m"},
		{"exp-form", sys + ` m(E) :- sys(E, S), S = 1000.`, "m"},
		{"trailing-zero-const", sys + ` m(E) :- sys(E, S), S = 81.40.`, "m"},
	}
	for _, c := range []string{"1e999999999", "1e-999999999", "1e+999999999", "1e1000000000", "1e-1000000000",
		"1e9223372036854775807", "1e-9223372036854775808", "1e99999999999999999999", "01", "-01", "1.", ".5", "+1",
		"0x10", "1_000", "1e", "1e+", "--1", "1.5.2", "00", "0.0", "-0.0", "0e5", "1E5", "123456789012345678901234567890",
		"-123456789012345678901234567890.123456789012345678901234567890", "0.000000000000000000000000000001"} {
		cases = append(cases, diffCase{"const-" + c, sys + ` m(E) :- sys(E, S), S < ` + c + `.`, "m"})
		cases = append(cases, diffCase{"fact-" + c, `n(` + c + `). m(X) :- n(X).`, "m"})
	}
	runCases(t, fx, cases)
}

// within/3 as the table builds it, from facts, then against every pair of instants in
// a real ledger.
func TestDifferential_Within(t *testing.T) {
	t.Parallel()
	pairs := []struct{ a, b, days string }{
		{noon, noon, "0"}, {noon, "2026-06-15T12:00:00.000000001Z", "0"}, {noon, sixLater, "0.25"}, {sixLater, noon, "0.25"},
		{noon, "2026-06-15T18:00:00.000000001Z", "0.25"}, {noon, "2026-06-15T17:59:59.999999999Z", "0.25"},
		{noon, "2026-06-16T12:00:00.000000000Z", "1"}, {noon, "2026-06-16T12:00:01.000000000Z", "1"},
		{"2026-05-01T07:00:00.000000000Z", "2026-06-26T07:00:00.000000000Z", "56"},
		{"2026-05-01T07:00:00.000000000Z", "2026-06-26T07:00:00.000000001Z", "56"},
		{noon, "2026-06-16T12:00:00.000000000Z", "1e0"}, {noon, "2026-06-15T12:00:00.000001000Z", "0.0000000000115740740740"},
		{"0001-01-01T00:00:00.000000000Z", "9999-12-31T23:59:59.999999999Z", "3700000"},
		{"0001-01-01T00:00:00.000000000Z", "9999-12-31T23:59:59.999999999Z", "3600000"},
		{"0001-01-01T00:00:00.000000000Z", "9999-12-31T23:59:59.999999999Z", "1e999999999"},
		{noon, "2026-06-15T12:00:00.000000001Z", "1e-999999999"}, {noon, noon, "1e-999999999"},
		{"2026-06-15", noon, "1"}, {"2026-06-15T12:00:00Z", noon, "1"}, {"2026-06-15T22:00:00.000000000+10:00", noon, "1"},
		{noon, "yesterday", "1"}, {noon, noon, "-1"}, {noon, noon, "-0"},
	}
	var facts strings.Builder
	for i, p := range pairs {
		fmt.Fprintf(&facts, "pair(%d, %q, %q, %s).\n", i, p.a, p.b, p.days)
	}
	// Days from data of the wrong kind, and the negated form.
	facts.WriteString("pair(100, \"" + noon + "\", \"" + noon + "\", \"1\").\npair(101, \"" + noon + "\", \"" + noon + "\", true).\n")
	fx := emptyFixture(t)
	fx.name = "within-table"
	runCases(t, fx, []diffCase{
		{"table", facts.String() + `holds(I) :- pair(I, A, B, D), within(A, B, D).`, "holds"},
		{"negated", facts.String() + `holds(I) :- pair(I, A, B, D), not within(A, B, D).`, "holds"},
		{"recursive", facts.String() + `
			t(I, A, B) :- pair(I, A, B, _).
			far(A, C) :- t(_, A, B), t(_, B, C), within(A, C, 400).
			far(A, D) :- far(A, C), t(_, C, D), within(A, D, 400).`, "far"},
		{"refused-bad-a", `p(1). h(X) :- p(X), within("2026-06-15", "` + noon + `", 1).`, "h"},
		{"refused-bad-b", `p(1). h(X) :- p(X), within("` + noon + `", "x", 1).`, "h"},
		{"refused-bad-days", `p(1). h(X) :- p(X), within("` + noon + `", "` + noon + `", "1").`, "h"},
		{"refused-negative-days", `p(1). h(X) :- p(X), within("` + noon + `", "` + noon + `", -1).`, "h"},
		{"refused-wrong-arity", `p(1). h(X) :- p(X), within("` + noon + `", "` + noon + `").`, "h"},
		{"refused-unbound-days", `p(1). h(X) :- p(X), within("` + noon + `", "` + noon + `", D).`, "h"},
		{"refused-anonymous", `p(1). h(X) :- p(X), within("` + noon + `", "` + noon + `", _).`, "h"},
		{"refused-in-head", `within("` + noon + `", "` + noon + `", 1).`, "within"},
		{"refused-declared", `within/3. h(X) :- within(X, X, 1).`, "h"},
		{"accepted-constants", `p(1). h(X) :- p(X), within("` + noon + `", "` + sixLater + `", 0.25).`, "h"},
	})

	rf := regimenDiffFixture(t)
	rf.name = "within-ledger"
	var cases []diffCase
	for _, days := range []string{"0", "0.5", "0.25", "1", "1.5", "7", "30", "56", "90.125", "400"} {
		cases = append(cases, diffCase{"near-" + days, fmt.Sprintf(
			`near(A, B) :- entry(_, _, A, _, _, _, _), entry(_, _, B, _, _, _, _), within(A, B, %s).`, days), "near"})
	}
	runCases(t, rf, cases)
}

// current/2 against fold.Snapshot at every instant, under tarski, as Check does it.
func TestDifferential_CheckCurrentEqualsSnapshot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.seedRegimen(t)
	h.admit(t, "health-db", "health.weight", "2026-06-15T06:00:00Z", `{"measured_at": "2026-06-15T06:00:00Z", "kg": 81}`)
	v, versions, _ := h.served(t)
	var ats []string
	for _, e := range v.Entries() {
		ats = append(ats, e.FactTime)
	}
	ats = append(ats, day("2025-12-31"), day("2026-05-01"), day("2026-07-15"), "2026-07-15T12:00:00.000000000Z", day("2027-01-01"))
	got, err := query.CurrentAtTarski(context.Background(), heldTexts(t)[bundledHash(t)], v, versions, h.defs, ats)
	if err != nil {
		t.Fatal(err)
	}
	for i, at := range ats {
		want := snapshotIDs(t, v, at, h.defs.Closes)
		if !same(got[i], want) {
			t.Errorf("tarski current at %s = %v, Snapshot = %v", at, got[i], want)
		}
		own, err := tryRun(v, versions, h.defs, fmt.Sprintf("instant(%q).", at), "current")
		if err != nil {
			t.Fatal(err)
		}
		if ownIDs := currentAt(own, at); !same(got[i], ownIDs) {
			t.Errorf("tarski current at %s = %v, own current = %v", at, got[i], ownIDs)
		}
	}
}

// Every held rule set text, each relation it defines, with the instants defined and
// without them.
func TestDifferential_HeldRuleSets(t *testing.T) {
	t.Parallel()
	h, v, versions := lazyFixture(t)
	fx := diffFixture{name: "held", v: v, versions: versions, defs: h.defs}
	texts := heldTexts(t)
	var hashes []string
	for hash := range texts {
		hashes = append(hashes, hash)
	}
	sort.Strings(hashes)
	if len(hashes) < 4 {
		t.Fatalf("%d held texts, want the bundled one and three in history", len(hashes))
	}
	for _, hash := range hashes {
		rs, ok, err := query.HeldRuleSet(hash)
		if err != nil || !ok {
			t.Fatalf("HeldRuleSet(%s): %v %v", hash, err, ok)
		}
		heads := map[string]bool{}
		for _, r := range rs.Program.Rules {
			heads[r.Head.Name] = true
		}
		var names []string
		for n := range heads {
			names = append(names, n)
		}
		sort.Strings(names)
		for _, n := range names {
			for _, with := range []bool{true, false} {
				text := ""
				if with {
					text = `instant(At) :- entry(_, _, At, _, _, _, _).`
				}
				name := fmt.Sprintf("%s-v%d-%s-%s-instants%v", hash[7:15], rs.Version, rs.Name, n, with)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					differ(t, fx, hash, diffCase{name, text, n}, 0)
				})
			}
		}
	}
}

// The size bound: both engines refuse, at the same relation with the same counts,
// across a ladder of bounds and several programs.
func TestDifferential_SizeBounds(t *testing.T) {
	t.Parallel()
	v, versions, _, _ := regimenFixture(t)
	fx := diffFixture{name: "bounds", v: v, versions: versions, defs: registry(t)}
	rf := regimenDiffFixture(t)
	rf.name = "bounds-ledger"
	programs := []diffCase{
		{"small-intermediate", `instant(At) :- entry(_, _, At, _, _, _, _). small(E) :- current(E, At), At > "2026-12-31T00:00:00.000000000Z".`, "small"},
		{"current", `instant(At) :- entry(_, _, At, _, _, _, _).`, "current"},
		{"in_force", `instant(At) :- entry(_, _, At, _, _, _, _).`, "in_force"},
		{"cross", `p(E, F, G) :- entry(E, _, _, _, _, _, _), entry(F, _, _, _, _, _, _), entry(G, _, _, _, _, _, _).`, "p"},
		{"base-only", ``, "entry"},
		{"no-rules-reached", `a(X) :- entry(X, _, _, _, _, _, _).`, "a"},
	}
	for _, f := range []diffFixture{fx, rf} {
		for _, c := range programs {
			for _, max := range []int{1, 2, 10, 50, 100, 199, 200, 201, 500, 1000, 2000, 5000, 20000, 1 << 20} {
				t.Run(fmt.Sprintf("%s/%s@%d", f.name, c.name, max), func(t *testing.T) {
					t.Parallel()
					differ(t, f, bundledHash(t), c, max)
				})
			}
		}
	}
}

// The refusals of plan_test and parse_test, and the things tarski's parser or compiler
// does differently from the package's: each must refuse under both, or the difference
// is recorded.
func TestDifferential_Refusals(t *testing.T) {
	t.Parallel()
	fx := emptyFixture(t)
	fx.name = "refusals"
	const e7 = `entry(X, _, _, _, _, _, _)`
	cases := []diffCase{
		// plan_test
		{"unknown-relation", `a(X) :- ` + e7 + `, nothing(X).`, "a"},
		{"arity", `a(X) :- entry(X, _, _).`, "a"},
		{"two-arities-for-one-head", `a(X) :- ` + e7 + `. a(X, Y) :- entry(X, Y, _, _, _, _, _).`, "a"},
		{"rule-over-base", `entry(X, "t", "", "", "", "", 1) :- attr(X, _, _).`, "entry"},
		{"declare-base", `attr/3.`, "attr"},
		{"unbound-head", `a(X, Y) :- ` + e7 + `.`, "a"},
		{"anonymous-head", `a(_) :- entry(_, _, _, _, _, _, _).`, "a"},
		{"unbound-in-negation", `a(X) :- ` + e7 + `, not attr(X, P, _), P != "".`, "a"},
		{"unbound-in-comparison", `a(X) :- ` + e7 + `, Y > 1.`, "a"},
		{"anonymous-in-comparison", `a(X) :- ` + e7 + `, _ > 1.`, "a"},
		{"unstratifiable", `a(X) :- ` + e7 + `, not b(X). b(X) :- ` + e7 + `, not a(X).`, "a"},
		{"bad-pointer", `a(X) :- attr(X, "drug", _).`, "a"},
		{"undeclared-pointer", `a(X) :- attr(X, "/dosage", _).`, "a"},
		{"pointer-not-a-string", `a(X) :- attr(X, 1, _).`, "a"},
		{"pointer-bool", `a(X) :- attr(X, true, _).`, "a"},
		{"pointer-in-negation", `a(X) :- ` + e7 + `, not attr(X, "/dosage", _).`, "a"},
		{"pointer-in-head-position", `attr(X, "/systolic", 1) :- ` + e7 + `.`, "attr"},
		{"instant-undefined", `a(X) :- current(X, _).`, "a"},
		{"instant-bare-date", `instant("2026-06-15"). a(X) :- current(X, _).`, "a"},
		{"instant-sloppy", `instant("2026-06-15T00:00:00Z"). a(X) :- current(X, _).`, "a"},
		{"instant-number", `instant(5). a(X) :- current(X, _).`, "a"},
		{"instant-ok", `instant("2026-06-15T00:00:00.000000000Z"). a(X) :- current(X, _).`, "a"},
		{"instant-in-body", `a(X) :- instant("2026-06-15").`, "a"},
		{"instant-in-negation", `a(X) :- ` + e7 + `, not instant("2026-06-15").`, "a"},
		{"instant-declared-twice", `instant/1. instant/1. instant("2026-06-15T00:00:00.000000000Z").`, "instant"},
		{"instant-arity-2", `instant(A, B) :- entry(A, B, _, _, _, _, _).`, "instant"},
		{"recursion", `r(X, Y) :- later(X, Y). r(X, Z) :- r(X, Y), later(Y, Z).`, "r"},
		{"declare-then-define", `x/2. x(A, B) :- before(A, B).`, "x"},
		{"declare-wrong-arity", `x/2. x(A) :- entry(A, _, _, _, _, _, _).`, "x"},
		{"declare-zero-arity", `a/0.`, "a"},
		{"derived-redefine-bundled", `current(A, B) :- before(A, B).`, "current"},
		{"redefine-bundled-wrong-arity", `current(A) :- entry(A, _, _, _, _, _, _).`, "current"},
		{"unused-undefined-relation-ref", `a(X) :- later(X, _).`, "a"},
		// parse_test
		{"parse-fact-and-rule", `a("x", 1). b(X) :- a(X, N), N > 0.`, "b"},
		{"parse-comment-declaration", "% a comment\ninstant/1.\nc(X) :- instant(X).", "c"},
		{"parse-negation-anonymous", `a(1, 2). b(1). d(X) :- a(X, _), not b(X).`, "d"},
		{"parse-every-operator", `a(1, 1). e(X) :- a(X, N), N = 1, N != 2, N < 3, N <= 3, N > 0, N >= 0.`, "e"},
		{"parse-booleans-escapes", `a("say \"hi\"", false). a(1, true). f(X) :- a(X, true), a("say \"hi\"", false).`, "f"},
		{"parse-negative-decimal", `a(1, -1.5). a(1, 2e3). g(X) :- a(X, -1.5), a(X, 2e3).`, "g"},
		{"parse-missing-period", `a(X) :- b(X)`, "a"},
		{"parse-unterminated-string", `a("x).`, "a"},
		{"parse-uppercase-relation", `A(X) :- b(X).`, "a"},
		{"parse-zero-arity-declaration", `a/0.`, "a"},
		{"parse-bad-operator", `a(X) :- b(X), X ~ 1.`, "a"},
		{"parse-term-for-atom", `a(X) :- 1.`, "a"},
		{"parse-empty-atom", `a() :- b(X).`, "a"},
		{"parse-second-line", "a(X) :- b(X).\nc(X) :- d(X), .", "a"},
		{"parse-fact-with-variable", `a(X).`, "a"},
		{"parse-fact-with-anonymous", `a(_).`, "a"},
		{"parse-bare-word", `a(b).`, "a"},
		{"parse-single-quote", `a('x').`, "a"},
		{"parse-unknown-escape", `a("\q").`, "a"},
		{"parse-escape-u", `a("\u00e9"). b(X) :- a(X).`, "b"},
		{"parse-escape-bad-u", `a("\u00zz").`, "a"},
		{"parse-newline-in-string", "a(\"x\ny\"). b(X) :- a(X).", "b"},
		{"parse-tab-in-string", "a(\"x\ty\"). b(X) :- a(X).", "b"},
		{"parse-nul-in-text", "a(\"x\x00y\"). b(X) :- a(X).", "b"},
		{"parse-invalid-utf8-string", "a(\"x\xffy\"). b(X) :- a(X).", "b"},
		{"parse-invalid-utf8-comment", "% \xff\na(1). b(X) :- a(X).", "b"},
		{"parse-bom", "\ufeffa(1). b(X) :- a(X).", "b"},
		{"parse-unicode-relation", `é(1).`, "é"},
		{"parse-unicode-variable", `a(É) :- entry(É, _, _, _, _, _, _).`, "a"},
		{"parse-digit-relation", `a1(1). b(X) :- a1(X).`, "b"},
		{"parse-underscore-relation", `a_b(1). b(X) :- a_b(X).`, "b"},
		{"parse-leading-underscore-relation", `_a(1).`, "_a"},
		{"parse-trailing-garbage", `a(1). }`, "a"},
		{"parse-double-neg", `a(X) :- entry(X, _, _, _, _, _, _), not not b(X).`, "a"},
		{"parse-neg-comparison", `a(X) :- entry(X, _, _, _, _, _, _), not X = 1.`, "a"},
		{"parse-negated-head", `not a(X) :- b(X).`, "a"},
		{"parse-two-heads", `a(X), b(X) :- entry(X, _, _, _, _, _, _).`, "a"},
		{"parse-colon-dash-only", `:- a(X).`, "a"},
		{"parse-rule-no-body", `a(1) :- .`, "a"},
		{"parse-var-eq-var", `a(X, Y) :- entry(X, Y, _, _, _, _, _), X = Y.`, "a"},
		{"parse-var-lt-var", `a(X, Y) :- entry(X, Y, _, _, _, _, _), X < Y.`, "a"},
		{"parse-const-eq-const", `a(1) :- 1 = 1.`, "a"},
		{"parse-equality-binds", `a(X) :- entry(Y, _, _, _, _, _, _), X = Y.`, "a"},
		{"parse-body-of-only-neg", `a(1) :- not b(1). b(2).`, "a"},
		{"parse-body-only-comparison", `a(1) :- 1 < 2.`, "a"},
		// engine differences to find
		{"aggregate-count", `n(C) :- C = count : { entry(_, _, _, _, _, _, _) }.`, "n"},
		{"aggregate-sum", `n(S) :- entry(_, _, _, _, _, _, V), S = sum V : { entry(_, _, _, _, _, _, V) }.`, "n"},
		{"aggregate-as-relation-name", `n(count) :- entry(_, _, _, _, _, _, _).`, "n"},
		{"builtin-prefix", `a(X) :- entry(X, _, _, _, _, _, _), prefix("a", X).`, "a"},
		{"builtin-suffix", `a(X) :- entry(X, _, _, _, _, _, _), suffix("a", X).`, "a"},
		{"builtin-contains", `a(X) :- entry(X, _, _, _, _, _, _), contains("a", X).`, "a"},
		{"builtin-prefix-negated", `a(X) :- entry(X, _, _, _, _, _, _), not prefix("a", X).`, "a"},
		{"user-relation-prefix-head", `prefix(X, Y) :- entry(X, Y, _, _, _, _, _).`, "prefix"},
		{"user-relation-prefix-fact", `prefix("a", "b"). a(X) :- prefix(X, _).`, "a"},
		{"user-relation-suffix-declared", `suffix/2.`, "suffix"},
		{"user-relation-contains-query", `contains(X, Y) :- entry(X, Y, _, _, _, _, _).`, "contains"},
		{"leading-zero-number", `a(01). b(X) :- a(X).`, "b"},
		{"leading-zero-comparison", `b(X) :- entry(X, _, _, _, _, _, V), V = 01.`, "b"},
		{"line-numbers", "a(1).\n\nb(X) :- a(X), nothing(X).", "b"},
		{"huge-arity", `a(` + strings.TrimSuffix(strings.Repeat("1,", 500), ",") + `).`, "a"},
		{"deep-chain", chain(60), "r60"},
		{"many-rules", manyRules(300), "r299"},
	}
	runCases(t, fx, cases)
}

func chain(n int) string {
	var b strings.Builder
	b.WriteString("r0(E) :- entry(E, _, _, _, _, _, _).\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "r%d(E) :- r%d(E).\n", i, i-1)
	}
	return b.String()
}

func manyRules(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "r%d(E) :- entry(E, _, _, _, _, _, %d).\n", i, i)
	}
	return b.String()
}

// Containers: the pointer refusals, under a registry that declares a container.
func TestDifferential_ContainerPointers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.admit(t, "bp-csv", "health.blood-pressure", "2026-06-15T07:00:00Z",
		`{"measured_at": "2026-06-15T07:00:00Z", "systolic": 120, "diastolic": 80, "cuff": {"arm": "left"}}`)
	v, versions, _ := h.served(t)
	fx := diffFixture{name: "container", v: v, versions: versions, defs: h.defs}
	runCases(t, fx, []diffCase{
		{"leaf", `arm(E, A) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/cuff/arm", A).`, "arm"},
		{"container", `cuff(E, A) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/cuff", A).`, "cuff"},
		{"root-pointer", `r(E, A) :- attr(E, "", A).`, "r"},
		{"escaped-pointer", `r(E, A) :- attr(E, "/a~1b", A).`, "r"},
		{"bad-escape-pointer", `r(E, A) :- attr(E, "/a~2", A).`, "r"},
	})

	two := diffFixture{name: "container-two-types", v: v, versions: versions, defs: registryOf(t, bpManifest, cuffObjectManifest)}
	runCases(t, two, []diffCase{
		{"container", `cuff(E, A) :- entry(E, "health.blood-pressure", _, _, _, _, _), attr(E, "/cuff", A).`, "cuff"},
	})
	mixed := diffFixture{name: "container-mixed", v: v, versions: versions, defs: registryOf(t, bpManifest, cuffNoteManifest)}
	runCases(t, mixed, []diffCase{
		{"field-and-container", `cuff(E, C) :- entry(E, "health.cuff-note", _, _, _, _, _), attr(E, "/cuff", C).`, "cuff"},
	})
}
