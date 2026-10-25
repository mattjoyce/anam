# Spans as filters: time-bounded access to the record

Draft for a card, 2026-10-25. Written on Matt's behalf from a design conversation about
Shearline. Not on the board yet: the board clone is not on this machine.

## Job story

When I give a principal a token to read sensitive facets, I want the grant to name a set of
time units as well as a set of facet types, so I can share a window of my record (this year's
blood pressure, last quarter's spend) without sharing the whole of it.

## Summary

Access to the record gains two dimensions beyond the three roles. A grant names *what* may be
read, as a resource prefix, and *when*, as a set of time units. A request is allowed when its
resource is under a granted prefix and every unit it asks for is covered by a granted unit.
Shearline decides; Anam resolves the calendar. Anam owns every word of its own vocabulary.

## Vocabulary

This feature uses the **derived span** of the Glossary: the lens a person names as `[from, to]`.
It never touches the **bucket span**, the minted checkpoint unit. Three new words:

- **Unit.** One calendar interval at one resolution, named by its ISO 8601 prefix in UTC:
  `2026` (year), `2026-06` (month), `2026-06-15` (day), `2026-06-15T09` (hour). The name is
  a string. Lexical order is chronological. A coarser unit is a string prefix of every finer
  unit under it, and that property is what makes a month grant cover its days.
- **Resolution.** The width of a unit: year, month, day, hour. A parameter, never inferred.
- **Span filter.** A derived span resolved to a set of units at one resolution, carried on a
  request so the policy can test it.

## Resolving a span to units

A function in `internal/span`, or beside it:

```go
// Units resolves the half-open interval [from, to) to the units of width w it touches,
// sorted. from and to are RFC 3339 instants; they are converted to UTC before resolution.
// A unit is touched when any instant of the interval falls inside it. Units returns an
// error when to is not after from, or when the interval would resolve to more than max
// units.
func Units(from, to time.Time, w Resolution, max int) ([]string, error)
```

Three rules, each a test:

1. **Half-open.** Start inclusive, end exclusive, so adjoining spans tile with no shared unit.
   `[2026-06-01, 2026-07-01)` at month resolution is exactly `[2026-06]`.
2. **Explicit resolution.** Six hours at day resolution is one day. A year at day resolution
   is 365 or 366 days. The caller says which; `Units` never guesses from the inputs.
3. **One zone.** Units are UTC, the zone of fact-time (`clock.Layout`). A span a person names
   in Sydney is converted before resolution. Grants are written in UTC units. Two zones would
   disagree at every edge, and the first refusal at ten in the morning would be a mystery.

## Resources

A resource is an opaque string with `/` as separator, canonical before the request is made.
Three kinds for now:

| Resource | Example | A grant prefix that covers it |
|---|---|---|
| a facet type | `facet/health.blood-pressure` | `facet/health.` covers every health type, present and future |
| a projection | `projection/recent-life` | `projection/` covers all |
| a read route | `read/timeline`, `read/changes` | `read/` covers all |

Actions are the verbs the daemon already has: `read`, `submit`, `mint`, `operate`.

## Policy facts

The three roles become standing facts, and the routes keep their role gate as today. The
policy adds what the role gate cannot say: which resources, and when.

```go
policy.Role("reader-bp-2026", "reader"),
policy.Grant("reader", "read", "read/"),                        // every read route
policy.Grant("reader-bp-2026", "read", "facet/health.blood-pressure"),
// span grants are Anam's own relation, read by Anam's own rule set:
anam.SpanGrant("reader-bp-2026", "2026"),                       // the year, as one unit
```

`span_grant/2` is declared in Anam's rule set and carried as standing facts through
`Config`, the same way `grants/3` is. Shearline's schema accepts a consumer relation when the
consumer's rule set declares it.

## Rules

Anam's rule set, loaded after `policy.RBAC()`:

```
% asked(U): the units the request's span filter resolved to. Request facts.
asked/1.

% span_grant(Role, Pat): the role may read units under Pat. Standing facts.
span_grant/2.

% covered(P, U): some role P holds grants a unit that is a prefix of U.
covered(P, U) :- principal(P), asked(U), holds(P, Role), span_grant(Role, Pat), prefix(Pat, U).

% missing(P): a unit was asked that nothing covers. "Every unit is covered" is written as
% "no unit is missing", the Datalog way to say for-all.
missing(P) :- principal(P), asked(U), not covered(P, U).

% allow/3 is the bundled rule plus one more condition. A request with no span filter asks
% no units, so missing/1 is empty and the rule reduces to the bundled one.
allow(P, A, R) :- permit(P, A, Pat), resource(R), prefix(Pat, R), not deny(P, A, R), not missing(P).
```

Stratified negation handles `not missing`; `missing` depends on `covered`, which depends on
nothing negated. A refusal's why-not names the first uncovered unit, which is the receipt a
person needs: "you asked for 2027-01 and hold 2026".

A request with no span filter is allowed on resource alone. That is deliberate: the
operator's own reads name no window. Whether a *reader* must always name one is a policy
choice for the standing facts, not the rules.

## Carrying the units on a request

Shearline's `Request.Context` holds one value per key. A span filter is many values. Two
ways through, in order:

1. **Now.** One `Decide` per unit, with the unit in context under a declared key `unit`, and
   `asked(U) :- context("unit", U).` Allow when all allow. A year of months is twelve
   decisions, each small. This is decision 5's answer in Shearline and costs nothing to build.
2. **If spans get large.** A declared context key with a *many* cardinality in Shearline, so
   `asked/1` arrives as request facts in one decision. A small, well-shaped extension to the
   `policy` package. The rules above do not change.

`Units` takes `max` so a request for ten years at hour resolution is refused at resolution
time, before it becomes 87,600 facts or 87,600 decisions.

## Where it sits in Anam

- Every read path builds a `policy.Request` beside the `receipt.Query` it already builds
  (card 92 made that one place: `queryOf` in `internal/read/parse.go`). Principal from the
  token, action `read`, resource from the route and type, span filter from `bounds`.
- `Decide` runs before the read. A deny is a 403 that carries `Decision.Why` rendered; an
  allow may attach `Decision.Proof` to the receipt, so a published read says why it was
  permitted as well as what it narrows.
- Standing facts come from the principals file, which already holds roles, extended with
  resource and span grants per principal. One `Load` at start and on reload; the `Policy`
  swaps behind a pointer, as `examples/logsvc` in Shearline shows.
- The policy base and the query base stay separate. A decision about a projection must not
  evaluate over the ledger it guards.

## Dependencies

- Shearline in `go.mod`: Matt's call under AGENTS.md.
- The engine split (Shearline's `datalog` into its own module) is independent of this card
  and can land before or after. Anam imports the engine directly for `anam query` either way.

## Pitfalls

- Mixed resolutions in one decision. A day grant and an hour request never match. `Units`
  takes one resolution; the policy layer refuses a request whose units are not all the same
  width.
- A zone other than UTC anywhere in a unit name.
- Writing span grants finer than needed. A year grant is one fact; the same year in days is
  365 facts and 365 lines of proof.
- Letting a resource name be a raw request path. Grants match by prefix; the resource must be
  the route the daemon matched and the type it resolved, never the URL.

## Not in scope

Attribute-based access over payload values (`attr/3` stays in the query engine, not the
policy). Per-field grants. Grants on bucket spans.

## Acceptance

- `Units` has table tests for the three rules, plus a leap day and a week across a month end.
- A reader with `facet/health.` and `2026` reads June 2026 blood pressure and is refused
  January 2027 with a why-not naming `2027-01`.
- A reader with no span grant and a request with no span filter is allowed on resource alone.
- The operator's existing reads are unchanged: same receipts, same hashes.
- A request whose units mix resolutions is refused before `Decide`.
