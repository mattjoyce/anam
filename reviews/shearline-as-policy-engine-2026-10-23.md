# Shearline as Anam's policy engine, assessed 2026-10-23

Read-only, Anam at master a71012c. Shearline was not at `/home/matt/Projects/shearline`; I read
`github.com/mattjoyce/shearline` at 08d15b8 (2026-10-22) from a scratch clone and ran its tests
there with the cached Go 1.25 toolchain: race and shuffle green, vet clean, coverage 95.9%
(datalog) and 92.3% (policy), 119 test functions.

## Verdict

Adopt later, at the trigger card 223 already names, and not before. Shearline does not move the
trigger, but it changes the flip's target. The decision page says the flip is "the bundled
Datalog engine over a policy base". That engine is `internal/query`, which builds its base from a
fold value and imports `internal/fold`, and `doctrine_test.go` lists `internal/query` among the
packages the policy may never reach, so the in-house flip as written cannot pass the doctrine
test. Shearline's `datalog` package is that engine with the fold cut out. When the flip comes, it
should be `shearline/datalog` with Anam's own schema (`principal/2`, `flag/2`, `request/3`), not
Anam's query package and not shearline's PARC layer.

Until then internal/policy stays as it is. Its seven functions are one-line comparisons; an
engine under them adds a module, a rule language and a string encoding of Anam's vocabulary for
no rule that needs them.

The one decision for Matt. Is shearline the canonical home of the engine, with `internal/query`
re-based on it (SPEC step 6), or does Anam keep its own copy? That settles the policy question
for free. If shearline is canonical, the flip is a schema and a rule file on a module Anam already
depends on. If Anam keeps its copy, the engines are already diverging (about a thousand differing
lines in `eval.go`, `plan.go` and `parse.go` after one day) and policy would be the first place
Anam depends on both.

## 1. What shearline is

Shearline is a Go library Matt wrote on 2026-10-22, Apache-2.0, standard library only, about 3,400
lines of code and 3,700 of tests, pre-1.0, no consumer yet. It answers one question: may this
principal take this action on this resource, in this context. You hand it facts and rules in a
small Datalog (facts like `role("ana", "reader").`, rules that derive new facts until nothing new
appears) and it derives the answer. An allowed request returns the proof that derived it, a
refused one returns why (which rule got how far, what it did not find), and every decision names
the rule set's version and sha256. It is not a service or a CLI; it reads no file, no network,
holds no state.

The `datalog` package is Anam's `internal/query` evaluator, cut from it (commit 6e0b211), plus a
base built from plain facts, `Base.With` for per-request facts, string built-ins (`prefix/2` and
two more), proofs, why-not, a step bound, and `RuleSet.Verify`, which re-parses the text so the
hash in a decision is the hash of the rules that ran. The `policy` package is a fixed vocabulary
over it (principal, action, resource, context; roles, inheritance, prefix grants, exact denies)
with a bundled `rbac.dl`. The policy it expresses is allow or deny over attributes by Datalog
rules; decision tables and payload attributes are out of scope by design. Two luminary panels and
an adversarial audit ran before the cut was a day old, each finding tied to its fixing commit. It
is still a one-day repo, and the README says the API may change.

## 2. Fit

All seven functions can be expressed with the same inputs only, but not without loss in
shearline's `policy` layer, whose schema is closed. Anam's rules are per-principal attributes
(source, types, any_type, receipts), not roles, and PARC has no `flag/2` or `source/2`, so each
principal becomes its own role and source and type fold into resource strings. `MaySubmitType`
and `ScopesOf` in that form, standing facts generated from the importers file:

```
role("scale", "scale").                                     % Role(p.Name, p.Name)
grants("scale", "submit",   "source/scale/type/health.weight/").   % one per Types
grants("scale", "register", "type/health.weight/").
role("conversation", "conversation").
grants("conversation", "submit", "source/conversation/type/").     % any_type: the open prefix, no register grant
role("docs", "docs").
grants("docs", "register", "type/record.document/").
grants("docs", "store-document", "library/").               % MayStoreDocument, the same ownership
```

`MaySubmitType(p, t)` is `Decide(ctx, Request{Principal: p.Name, Action: "submit", Resource:
"source/" + p.Source + "/type/" + t + "/"})`. `ScopesOf(p)` is one `Permitted(ctx, p.Name,
"submit")` call whose prefixes build the `Scopes` value the gate holds, so the engine never runs
inside the commit. The trailing separators (DESIGN.md decision 1) are the loss: Anam's types
become path strings, and a missing slash is a wrong allow (`health.weight` would match
`health.weightx`).

`ListScope` and its clamp read better over `shearline/datalog` with Anam's own schema, the one
card 223 named. Schema: `principal(Name, Role)`, `flag(Name, Flag)`, `source(Name, Source)`,
`request(Name, Action, Asked)`, never `attr/3`. The rule file, `anam-policy/1`:

```
% ListScope: the receipts a principal lists. A recording token lists only its own.
reader_scope(P, P)   :- request(P, A, R), flag(P, "records").
reader_scope(P, "*") :- request(P, A, R), not flag(P, "records").
source_scope(P, S)   :- request(P, A, R), principal(P, "importer"), source(P, S).
source_scope(P, "*") :- request(P, A, R), not principal(P, "importer").
list_scope(P, Reader, Source) :- reader_scope(P, Reader), source_scope(P, Source).

% Scope.Receipts(asked): the clamp. A recording token asked for another reader's is refused.
list_refused(P, Asked) :- request(P, "list-receipts", Asked), flag(P, "records"),
                          Asked != P, Asked != "*".
```

The handler reads `list_scope` and `list_refused` as relations. In the PARC layer the clamp is a
per-principal grant (`grants("assistant", "list-receipts", "receipts/assistant/")`) read back by
parsing the prefix, which works and is uglier. The other functions are one or two rules of the
same shape; `Held` takes the hold set as request facts through `With`. `TimelineRead`'s refusal
text, `ReaderOf` and the `Scope` and `Scopes` values stay in Go either way: return shapes, not
rules.

## 3. The seam

In-process library. `internal/policy` keeps its signatures; each function builds request facts
from `config.Principal` and the request value, evaluates against a `Policy` built once at start
from the importers file, and reads a relation. The hold set and the request's fields are
per-request facts. The rule file is embedded with `go:embed` like `anam.dl` and named by
`datalog.NewRuleSet("anam-policy", 1, text)`, whose `ID()` is `name/version sha256:<hex>`, the
shape `receipt.Program` already stores for `anam.dl` (`rule_set`, `rule_set_version`,
`rule_set_hash`). A receipt gains the same three fields for the policy that admitted the read,
taken from the `Policy` value that decided, not a global.

Reload without a restart is possible, since a `Policy` is an immutable value: `Load` a new one,
which fails closed and leaves the old in place, and swap it as the daemon swaps Definitions. I
would not: an embedded rule file changes with the binary, which is what makes its hash a version.

The doctrine test drops every import outside the Anam module (`moduleImports` cuts on the module
prefix), so today it would not see shearline at all. It needs an allow-list of external imports
(`shearline/datalog`, nothing else), a pin on the schema's relation names (`principal`, `flag`,
`source`, `request`, `hold`, and a failure if `attr` or anything built from a facet appears), and
a check that every `Fact` the package builds comes from a `config.Principal` or `Request` field.
Shearline's contract (no file, no network, stdlib only) covers the rest, and an empty `require`
in its `go.mod` is checkable.

## 4. Against the in-house flip

Hickey. The in-house flip is the more braided: `internal/query` complects the evaluator with the
fold-backed base, Definitions and `attr/3`, which is why the doctrine test bans it. Shearline's
`datalog` is that evaluator with the braid cut. Its `policy` layer would add a braid of its own
for Anam, PARC's vocabulary and prefix strings carrying source and type, so Anam should import
`datalog` alone. The real complecting cost today is two forks of one engine; every fix lands in
one and not the other.

Armstrong. Static Go fails at compile and in a table test, and has no runtime failure mode.
Shearline fails closed at every stage: a hash that does not match its text, an unsafe rule, a
negation cycle or an undeclared relation is refused at `Load`, so a bad file never serves;
`Decide` never returns allow with an error; a bound crossed is an error, never a partial answer.
A bad rule file can open access, since a rule set may add `allow` rules and the only promise that
survives any set is deny by absence. That is the same exposure as an edited `policy.go`, with the
same safety net, the table of cases. The engine runs at the handlers, above the ledger; keep the
gate's `Scopes` a precomputed value so no evaluation sits inside the admission transaction.
Evaluation is bounded and per-request seeded, microseconds for tens of principals.

Cost. A `require` line with no transitive dependencies, pinned to a commit until there is a tag.
The in-house flip costs the split of `internal/query` that shearline has already done. The
maintenance cost is the fork, Anam on its own copy for queries and on shearline for policy, until
SPEC step 6 lands.

What each gives. Shearline gives proofs and why-not in every decision, a verified hash per
decision, a race-safe shared `Policy` value, declared context keys, and a second consumer
(Ductile) sharing the fixes. Static Go gives a typed request, return shapes richer than a
boolean, the compiler as the rule checker, no string encoding and no module. The in-house flip
gives nothing shearline lacks.

Matt's own. The stdlib has no Datalog; the module costs a `go.mod` line and no transitive
dependencies, and its maintenance is already Matt's. So the rule's question changes from whether
to add a module to which copy of the engine is canonical. It is still Matt's call.

## 5. When

The trigger stands: more than about a dozen rules, a second principal class, or the first rule
that must read beyond the request and the file. Shearline lowers the flip's cost and fixes its
target. One thing fits PARC directly: the second principal class the decision page imagines, a
clinician's reader with a projection grant per role, is roles and grants over `projection/`
prefixes exactly.

Smallest step that proves fit without committing Anam: an agreement test in shearline's own repo
that copies the principals and every row of Anam's `TestAllow`, `TestSubmitAndRegister`,
`TestReadsAndReceipts` and `TestListScope` as data, loads them through `datalog` with the schema
and rules in section 2, and asserts the same answers. No change to Anam's `go.mod`, no decision
needed. If it holds, and when Matt says yes to the module, the same test moves into
`internal/policy/agreement_test.go` against the live functions, and the flip is replacing bodies
behind unchanged signatures with that test green.

## 6. Pitfalls

Shearline is pre-1.0 and a day old: pin a commit, not a branch, and expect the API to move while
Ductile exercises it. The unraid Docker build cannot use a local `replace` directive, so
co-development means pushing shearline first. The separator, seeding and doctrine-test traps are
in sections 2 and 3.
