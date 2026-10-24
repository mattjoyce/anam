# Asking the record a question

This page is the reference for Anam's query surface, and a few questions to copy. It documents
only what Anam adds to Datalog. For the language itself, read a standard account such as
[Datalog](https://en.wikipedia.org/wiki/Datalog); Anam's dialect is small and its syntax block is in
the catalogue below.

Two other places say the same things from the other side. The running daemon's `GET /v1/query` is the
catalogue for the build you are talking to, and cannot drift from it, so read it before writing a
program against a new type. [ARCHITECTURE.md](ARCHITECTURE.md) §9 says why the query exists and where
it sits among the reads. The worked questions below are run by `internal/query/docs_test.go` against
a real ledger, so an answer on this page is an answer the evaluator gives.

## What a query is

A query is a Datalog program run over a **served value**. A projection decides which whole facets the
program may see, as it does for a timeline read. The program then relates them. It reads no store, since the
payloads come from the served copy. It moves no watermark and writes nothing. The answer is one sorted
relation, filed with a receipt.

The projection matters. A log-shaped projection serves every version of its types, so `current/2` can
tell you what was in force on a date. A snapshot-shaped one (the bundled `meds`) has already taken the
latest of each series, so a question about the past finds nothing there to look at. Use a log-shaped
projection for history. The bundled ones are `activity`, `attention`, `body`, `bp`, `clinical`, `doses`, `intake`, `meds` and `sleep`, and
`GET /v1/projections` lists them with the registered ones. A type served by none of them has no query route;
`GET /v1/coverage` names, for each type, the projections that serve it. Others are registered with
`POST /v1/projections`.

## Asking without a registered projection

For a one-off question, the request can carry the projection itself. In place of a name, `projection`
takes an object with the fields of a manifest, without its name, version or description:

```json
{
  "projection": {
    "types": ["health.weight", "health.blood-pressure."],
    "statuses": ["admitted", "endorsed"],
    "max_classification": "Sensitive",
    "from": "2026-01-01T00:00:00.000000000Z",
    "shape": "log"
  },
  "text": "heavy(E, Kg) :- entry(E, \"health.weight\", _, _, _, _, _), attr(E, \"/kg\", Kg), Kg > 80.",
  "relation": "heavy"
}
```

`types`, `statuses` and `max_classification` are required, as they are in a manifest. `from`, `to` and
`shape` are optional, and `shape` is `log` when left out. A `types` entry ending in `.` admits every type
under it. The projection loader that checks a registered manifest checks this one, so a mistake is a 400
that names the field (`types`, `statuses`, `max_classification`, `from`, `to`, `shape`, or an unknown
key), and so is a type that no Definition publishes. Nothing is read before it is refused.

An inline projection has no name and no version. It is known by its content hash, worked out the way a
registered template's is, so the same object sent twice is the same projection. In the answer and the
receipt it appears as `inline` at version 1, and the receipt carries `template_inline: true` and the
hash in `template_hash`. The body is filed in the operational store under that hash, so
`GET /v1/receipts/{hash}/verify` finds it and replays the answer, after a restart too. A dry run files
no body. Every role may already read every registered projection, so an inline one lets a reader see
nothing it could not already see.

The CLI still takes a name. Reading through a saved selection's code in place of a projection is a
possible later slice and is not built.

## Running one

| Surface | How |
|---|---|
| CLI | `anam query run --projection bp -e '<program>'`, or `-f file`, or stdin. `--relation` names the answer, by default the head of the last rule. `--dry-run` derives and files nothing, `--as-of` and `--w` pick the knowledge, `--json` prints the receipt too. `anam query describe` prints the catalogue. With `ANAM_SERVER` set both ask the daemon. |
| `POST /v1/query` | JSON body `projection` (required: a name, or a template object, see below), `text`, `relation`, `as_of` or `w` (at most one), `zone` (for a bare date in `as_of`), `dry_run`. 201 when the receipt was filed, 200 for a dry run. |
| `GET /v1/query` | The catalogue, with no parameters. |

All three take a reader's, importer's or operator's token. A body with a field outside the list is a
400.

Failures say what to change. 400 is a program that cannot run (a syntax error, an unknown relation, an
unsafe variable, a pointer no Definition declares, an unknown projection). 413 is a derivation or an
answer over `max_read_bytes`, with `narrow` naming the projection and the program. 409 is a receipt
verify whose receipt names a template version, a ledger or a rule set this binary does not hold.

The answer to `POST /v1/query` carries `w` (the watermark it stood on), `ledger_id`, `projection`
(name, version, how many facets it served, its hash), `rules` (name, version, hash), `answer`
(`relation` and `tuples`), `hash` of the answer, `warnings`, `receipt_hash`, `receipt` and `written`.
`warnings` names a pointer that some versions of a type declare and others do not. A facet admitted
under a version without it has no `attr` for it, so the query misses it silently.

## Receipts

A query answer is filed as a served value and a receipt. The receipt records the ledger, the watermark,
the projection and its hash (an inline projection is marked `template_inline`), the definition-set version, the hash of the answer, and a `program`
block with the rule set (`rule_set`, `rule_set_version`, `rule_set_hash`), the text as written and the
relation answered. `GET /v1/receipts/{hash}/verify` folds the ledger at that watermark, serves the
projection, runs the same program and compares hashes. It reports a difference if the record changed
underneath, and it refuses with 409, saying the rule set has moved, when no rule set this binary holds
has the hash the receipt names, since another rule set cannot re-derive the answer. A dry run derives
the same receipt and files nothing.

Verify picks the rule set by the receipt's `rule_set_hash`, never by `rule_set_version` alone. The
binary holds its bundled rules and every earlier text a receipt may name, so a receipt cut under rule
set v1 still verifies under a binary that bundles v2. The versions are in
[the bundled rules](#the-bundled-rules).

## The relations

Four relations are known before the program says anything.

| Relation | Arguments | Meaning |
|---|---|---|
| `entry/7` | `E, Type, FactTime, Series, Status, Source, DefinitionVersion` | One served facet's envelope. `Series` is `""` for a facet that stands alone. `DefinitionVersion` is the version the payload was admitted under. |
| `attr/3` | `E, Pointer, Value` | Every scalar leaf of a served payload, by its JSON pointer (RFC 6901, so `/cuff/arm`, `/tags/0`). A null is no value, and a container is not a value either: ask for the leaves beneath it. Numbers are numbers, strings strings, `true` and `false` booleans. |
| `closes/1` | `Type` | The types whose Definition closes a series. |
| `instant/1` | `At` | A parameter. The program defines it, as facts or as a rule, and the bundled rules read it. |

A base relation cannot be the head of a rule. `instant/1` is declared by the bundled set and defined
by the query.

### Fact-times

A fact-time is the fixed-width lexeme `2006-01-02T15:04:05.000000000Z`, in UTC. Strings compare
lexically, so fact-times compare chronologically. That is also why a constant in `instant/1` must be a
whole lexeme. A bare date sorts before every entry of its day and asks a question nobody meant, so a
program that writes `instant("2026-06-15")` is refused, and the message gives the lexeme it should have
been.

### Numbers

A number is the exact decimal the record holds, and it answers as that decimal written out: an
integer in full (`1792269840000`, not `1.79226984e+12`). One value has one form, so `1` and `1.0` are
one number and answer the same. Numbers compare by value and exactly: `9007199254740992` and
`9007199254740993` are two numbers, though a float64 holds them as one. A number whose written-out
form would run past 64 digits answers in scientific form, which is exact as well.

### Pointers

A constant pointer in `attr/3` must be a field some Definition declares. `attr(E, "/systolic", V)`
runs. `attr(E, "/nope", V)` is refused before anything is read, because a name no type declares could
only ever be empty. A pointer that names a container is refused too, when every Definition that
declares it declares it as one: `attr` holds leaves and a container holds none, so the program would
answer empty forever and say nothing. The refusal names the leaves to ask for instead, so
`attr(E, "/cuff", V)` is refused with `/cuff/arm` in the message. A pointer that names a container in
one type and a field in another is not refused: the field binds for the type that has one. A variable
in the pointer position reaches every leaf, including the ones of a type that declares no schema.

The catalogue lists each type's pointers by Definition version, so write a program against it.

### Types by prefix

A type pattern is an exact type or a prefix ending in `.`, such as `health.medication.`. The query
language has no prefix operator. A prefix is a pair of lexical bounds. One is above the prefix, and one below it
with the closing `.` raised to `/`, the next byte. `T > "health.medication.", T < "health.medication/"`
holds for every type that starts with the prefix and is longer, and for nothing else. The selection
builder writes it this way, and so do the bundled edge rules for a pointer.

## The bundled rules

The rule set `anam` ships with the binary. Its text is `internal/query/rules/anam.dl`, with a comment
per rule, and the catalogue returns it. Its version and hash go into every receipt.

It is at version 2 (card 212, which added `in_force/2`). Every earlier text a receipt may name is kept
under `internal/query/rules/history/`, byte for byte, and a text there is never edited. Version 1 was
served as three different texts, because the edge rules landed on 2026-10-21 without a bump and the
`record.note` to `record.edge` rename followed the same day. The hash told them apart and the version
did not, which is why verify goes by the hash. A change to `anam.dl` moves the version and puts the
text it replaces in the history first.

| Rule | Holds when |
|---|---|
| `before(E, At)` | E's fact-time is at or before `At`. |
| `later(E, F)` | F follows E in one series: later in time, or at the same instant with the greater id. The tie the fold breaks. |
| `displaced(E, At)` | A later statement about E's series stands at or before `At`. |
| `latest(E, At)` | E is the latest of its series at `At`. |
| `closed(E, At)` | E is the latest of its series at `At`, and its type closes the series. |
| `current(E, At)` | E is what the record last said at `At`: at or before it, not displaced, not closed. A facet with no series is current from its fact-time on. A test holds this equal to `fold.Snapshot`. |
| `in_force(E, F)` | E is current at the fact-time of F: the statement standing when F happened. It holds for an F whose fact-time the query names in `instant/1`, and for no other, which is what bounds its cost. A test holds it equal to `current/2` at the blood-pressure readings' instants. |
| `edge_to(E, F)` | The `record.edge` E names F on its `to` side (its `about` side, for a note). |
| `edge_from(E, F)` | The edge E names F on its `from` side. |
| `linked(E, F)` | The edge E names F on either side. The edges touching F are the E with `linked(E, F)`. |

`before`, `displaced`, `latest`, `closed`, `current` and `in_force` need `instant/1` to hold something. A query
that reaches them with `instant/1` undefined is refused, since reached silently it would be empty and
the answer wrong. A query that never reaches them needs no definition.

The edge rules read `attr/3`. A program that reads only `entry/7` and `closes/1` does not pay for it.
`attr/3` has a tuple for every scalar leaf of every served payload, so the evaluator builds it only
when the relation the program answers reaches `attr`, directly or through a bundled rule.
The answer is the same either way.

## Builtins

A builtin is a predicate the evaluator answers in Go, because the language has no arithmetic to write
it in. It is written like an atom in a rule body, and it is a guard, as a comparison is: it names no
relation, defines nothing, and is true or false of values already bound. Every argument a builtin
requires must be bound by a positive body atom, and the plan check refuses a program that leaves one
unbound, naming the predicate and the argument. `GET /v1/query` and `anam query describe` list the
builtins with their arguments. `not` in front of one is allowed, since everything is bound by then.

| Builtin | True when |
|---|---|
| `within(A, B, Days)` | The fact-times `A` and `B` are at most `Days` days apart, in either order. `A` and `B` are whole fact-time lexemes. `Days` is a number that is not negative and may be a decimal: `0.25` is six hours. A day is 24 hours of UTC, since a fact-time is one UTC instant, so a clock change in a local zone does not lengthen or shorten it. The comparison is exact to the nanosecond. |

A constant that can never be right is refused at plan time: a bare date for `A` or `B`, a string for
`Days`, a negative `Days`. A value bound from data that is not what the builtin takes makes it false
and refuses nothing, since the program was right to ask and the record is what it is.

`within` cannot be written as a rule, and the Soufflé check in `scripts/souffle-diff.py` does not
cover it. That check holds the rules to a second engine. The builtin is held by table tests and by a
brute-force check against every pair of instants in a real ledger.

## What the language does not do

- No aggregates. There is no `count`, `sum` or `max`. A count is the number of tuples in the answer,
  which the caller reads, and the receipt pins.
- No function symbols or arithmetic, so a program always terminates. The builtins above are the one
  exception to reading it that way: they test values, and bind nothing, so they cannot make it run
  forever.
- Negation must be stratified, so a relation may not depend on its own negation.
- Every variable in a head, a negation or a comparison appears in a positive body atom.
- Different kinds never compare. A string is neither less than nor equal to a number.

## Worked questions

The record these run against is a small one. Ramipril started on 1 March 2026 and had a dose change
on 15 May. Vitamin D3 + K2 started on 1 May, stopped on 15 July, and began a second course on 10
August. Blood-pressure readings were taken on 15 February, 10 April, 1 June, 20 July and 20 August,
and the April one is the high one at 146. A note, "Before the dentist.", is about the April reading.
For the alcohol question, the record also holds three `health.intake` alcohol facts (two red wines on the
evening of 21 October, a beer on the evening of 22 and another on the evening of 23), a flat white on the
morning of the 22nd, and two nights' sleep, which began at 12:30 UTC on the 21st and at 12:00 UTC on the 22nd.
The medication types are `health.medication.started`, `.dose-changed` and `.stopped`, and each course
of a drug is one series.

### A reading above a bound

```datalog question=high-readings relation=high
high(At, Systolic) :- entry(R, "health.blood-pressure", At, _, _, _, _),
                      attr(R, "/systolic", Systolic), Systolic > 140.
```

```answer question=high-readings
2026-04-10T07:00:00.000000000Z 146
```

### What was current on a date

Name the date as an instant, then ask `current/2`. In this record the medication facets that state a drug carry it at
`/drug`.

```datalog question=on-a-date relation=taking
instant("2026-07-20T00:00:00.000000000Z").
regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
taking(Drug) :- instant(At), current(E, At), entry(E, T, _, _, _, _, _),
                regimen_type(T), attr(E, "/drug", Drug).
```

```answer question=on-a-date
ramipril
```

Vitamin D3 + K2 is missing because its first course was stopped on 15 July, and the second has not
begun.

### The regimen at each reading

One question over many instants. `instant/1` can be a rule over `entry/7`.

```datalog question=regimen-at-each-reading relation=regimen
instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
regimen(At, Drug) :- entry(_, "health.blood-pressure", At, _, _, _, _), current(E, At),
                     entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", Drug).
```

```answer question=regimen-at-each-reading
2026-04-10T07:00:00.000000000Z ramipril
2026-06-01T07:00:00.000000000Z ramipril
2026-06-01T07:00:00.000000000Z vitamin d3 + k2
2026-07-20T07:00:00.000000000Z ramipril
2026-08-20T07:00:00.000000000Z ramipril
2026-08-20T07:00:00.000000000Z vitamin d3 + k2
```

### The regimen in force at each reading, with `in_force/2`

The same question with the join to the instant written by the rules. `in_force(E, R)` says E was
current when the reading R happened, so there is no `At` to carry. `instant/1` still has to name the
readings' fact-times, since that is what bounds the work.

```datalog question=regimen-in-force relation=regimen
instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
regimen(At, Drug) :- entry(R, "health.blood-pressure", At, _, _, _, _), in_force(E, R),
                     entry(E, T, _, _, _, _, _), regimen_type(T), attr(E, "/drug", Drug).
```

```answer question=regimen-in-force
2026-04-10T07:00:00.000000000Z ramipril
2026-06-01T07:00:00.000000000Z ramipril
2026-06-01T07:00:00.000000000Z vitamin d3 + k2
2026-07-20T07:00:00.000000000Z ramipril
2026-08-20T07:00:00.000000000Z ramipril
2026-08-20T07:00:00.000000000Z vitamin d3 + k2
```

These are the rows of the previous question, and the docs test checks that the two answers on this page are the same.

### Readings within 56 days of a dose change

`within/3` takes the two fact-times and the number of days, so there are no bounds to work out by
hand. Each row is a reading and the dose change it is near. Ramipril's dose changed on 15 May, which is
35 days after the April reading and 17 before the June one, and 66 before the July one, so July is out.

```datalog question=near-a-dose relation=near_dose
near_dose(At, ChangedAt) :- entry(_, "health.blood-pressure", At, _, _, _, _),
                            entry(_, "health.medication.dose-changed", ChangedAt, _, _, _, _),
                            within(At, ChangedAt, 56).
```

```answer question=near-a-dose
2026-04-10T07:00:00.000000000Z 2026-05-15T00:00:00.000000000Z
2026-06-01T07:00:00.000000000Z 2026-05-15T00:00:00.000000000Z
```

### The latest statement of each series

`latest/2` includes a series' closing statement. Here every regimen's latest statement, as of a date
after the record ends.

```datalog question=latest-per-series relation=last
instant("2027-01-01T00:00:00.000000000Z").
last(Series, Type) :- instant(At), latest(E, At), entry(E, Type, _, Series, _, _, _).
```

```answer question=latest-per-series
med.regimen:15:vitamin d3 + k2:1:1 health.medication.stopped
med.regimen:15:vitamin d3 + k2:1:2 health.medication.started
med.regimen:8:ramipril:1:1 health.medication.dose-changed
```

The series names are composed by the Definition (the drug's length, the drug, the course), so match
them through `entry/7` by type or by a rule, and treat the text as opaque.

### A count over a month

The language has no `count`, so ask for the rows and count them. These are the readings from June to
August, three of them.

```datalog question=summer-readings relation=summer
summer(At) :- entry(_, "health.blood-pressure", At, _, _, _, _),
              At >= "2026-06-01T00:00:00.000000000Z", At < "2026-09-01T00:00:00.000000000Z".
```

```answer question=summer-readings
2026-06-01T07:00:00.000000000Z
2026-07-20T07:00:00.000000000Z
2026-08-20T07:00:00.000000000Z
```

The upper bound is exclusive so that months tile.

### An absence

The readings in summer at whose moment no regimen statement names vitamin D3 + K2. An absence is
negation over a relation you define, since negation can only test what is bound.

```datalog question=without-d3k2 relation=without
instant(At) :- entry(_, "health.blood-pressure", At, _, _, _, _).
regimen_type("health.medication.started"). regimen_type("health.medication.dose-changed").
on_d3k2(At) :- current(E, At), entry(E, T, _, _, _, _, _), regimen_type(T),
               attr(E, "/drug", "vitamin d3 + k2").
without(At) :- entry(_, "health.blood-pressure", At, _, _, _, _),
               At >= "2026-06-01T00:00:00.000000000Z", At < "2026-09-01T00:00:00.000000000Z",
               not on_d3k2(At).
```

```answer question=without-d3k2
2026-07-20T07:00:00.000000000Z
```

### The words said about a reading

Notes are `record.edge` facets, so `linked/2` finds them. The words are in the edge's payload.

```datalog question=notes-on-readings relation=noted
noted(At, Text) :- entry(R, "health.blood-pressure", At, _, _, _, _), linked(Edge, R),
                   attr(Edge, "/text", Text).
```

```answer question=notes-on-readings
2026-04-10T07:00:00.000000000Z Before the dentist.
```

### Alcohol before a sleep episode

Each alcohol intake with the next sleep episode that began after it. `health.intake` keeps what kind of
intake it was at `/kind` and which drink at `/drink`, so the flat white is left out by asking for
`alcohol`. The next sleep is the earliest later start, found by negation over a relation you define:
a start is skipped for an intake when another start falls between them. The beer on the 23rd is
followed by no sleep in the record, so it has no row. Answers are the intake's instant, the drink and
the sleep's start.

```datalog question=alcohol-before-sleep relation=before_sleep
alcohol(At, Drink) :- entry(A, "health.intake", At, _, _, _, _),
                      attr(A, "/kind", "alcohol"), attr(A, "/drink", Drink).
sleep_start(At) :- entry(_, "health.sleep-episode", At, _, _, _, _).
skipped(At, Next) :- alcohol(At, _), sleep_start(Next), sleep_start(Other),
                     At < Next, At < Other, Other < Next.
before_sleep(At, Drink, Next) :- alcohol(At, Drink), sleep_start(Next),
                                 At < Next, not skipped(At, Next).
```

```answer question=alcohol-before-sleep
2026-10-21T09:00:00.000000000Z red-wine 2026-10-21T12:30:00.000000000Z
2026-10-22T08:30:00.000000000Z beer 2026-10-22T12:00:00.000000000Z
```

This says the alcohol came before the sleep, not how long before. The wine and the beer were each
followed by the next sleep, and the answer would be the same if that sleep were a week away.

To ask for a sleep within six hours, say so with `within/3`. A day is 24 hours, so six hours is
`0.25`. The rows are the same two here, since each sleep began three and a half hours after its
drink. `0.125`, three hours, would leave neither. The sleep is no longer the next one: it is any
sleep that began within six hours after the drink, and a night with two such starts would give two
rows.

```datalog question=alcohol-within-six-hours relation=within_six_hours
alcohol(At, Drink) :- entry(A, "health.intake", At, _, _, _, _),
                      attr(A, "/kind", "alcohol"), attr(A, "/drink", Drink).
sleep_start(At) :- entry(_, "health.sleep-episode", At, _, _, _, _).
within_six_hours(At, Drink, Next) :- alcohol(At, Drink), sleep_start(Next),
                                     At < Next, within(At, Next, 0.25).
```

```answer question=alcohol-within-six-hours
2026-10-21T09:00:00.000000000Z red-wine 2026-10-21T12:30:00.000000000Z
2026-10-22T08:30:00.000000000Z beer 2026-10-22T12:00:00.000000000Z
```

### Every medication event, by prefix

The type prefix as lexical bounds, in a window.

```datalog question=medication-events relation=events
events(At, Type) :- entry(_, Type, At, _, _, _, _),
                    Type > "health.medication.", Type < "health.medication/",
                    At >= "2026-07-01T00:00:00.000000000Z", At < "2026-09-01T00:00:00.000000000Z".
```

```answer question=medication-events
2026-07-15T00:00:00.000000000Z health.medication.stopped
2026-08-10T00:00:00.000000000Z health.medication.started
```

### Which types close a series

```datalog question=closing-types relation=closing
closing(Type) :- closes(Type).
```

```answer question=closing-types
health.medication.stopped
```

### Programs that are refused

A pointer no Definition declares.

```datalog refused=unknown-pointer relation=nope
nope(V) :- attr(_, "/nope", V).
```

```error refused=unknown-pointer
attr: no Definition declares a field at /nope
```

The message goes on to list the types the daemon holds.

A bare date as an instant.

```datalog refused=bare-date relation=now
instant("2026-07-20").
now(E) :- instant(At), current(E, At).
```

```error refused=bare-date
instant 2026-07-20 is not a fact-time
```

A builtin with an argument nothing binds.

```datalog refused=unbound-within relation=near
near(At) :- entry(_, "health.blood-pressure", At, _, _, _, _), within(At, Other, 56).
```

```error refused=unbound-within
within: argument B is Other, which no positive body atom binds
```

A rule that reaches `current/2` with no instants to ask about.

```datalog refused=no-instants relation=now
now(E) :- current(E, At).
```

```error refused=no-instants
declared but has no rules or facts
```

## The selection builder's programs

The Workbench's selection page writes a program from the criteria a person adds, and the
program text is what the selection is handed on as. Every one defines `pick/1`, the facets selected,
over `entry/7`. It is one rule per pair of a type pattern and a source the criteria name, so the rules'
union reads "any of these types, from any of these sources". A fact-time window, a standing, and
the facets unticked by hand are conjuncts of each rule. A facet ticked by hand is a fact, `pick("<id>")`;
an unticked one is `unpick("<id>")`, which each rule reads as `not unpick(E)`. A prefix is the pair of
lexical bounds above. The program never reads `attr/3`, so it names no pointer and the evaluator
builds none.

The page draws each facet in the list, the basket and the link form as its label when the type's
Definition declares one (`handling.label`, read by `def.Definition.Label`), and as its type and time
when it does not. The label is for the person's eye: the program, the counts and the addresses know
nothing of it.

```datalog selection=summer-medication relation=pick
% An Anam selection (card 199): pick/1 holds the facets it selects.
% dates read in Australia/Sydney
pick/1.
pick(E) :- entry(E, T, At, _, _, S, _),
    T > "health.medication.", T < "health.medication/",
    S = "self-log",
    At >= "2026-05-31T14:00:00.000000000Z", At < "2026-08-31T14:00:00.000000000Z",
    not unpick(E).
pick("w1").
unpick("m4").
```

A bare date on the page is a day in the reader's zone, as it is everywhere else (the `zone` parameter,
below), so "from 1 June to 31 August" typed in Sydney becomes the instants of Sydney's midnights, and the
program says so in the one comment line under its header: `% dates read in Australia/Sydney`. The
instants mean the same in any zone; the comment records what the dates meant to the person who chose
them, and `selection.Parse` ignores it as it ignores every comment. A filter with no bound writes no
comment. Saving keeps the line, so the same instants read in another zone are another saved selection;
a program saved before the line existed has none, and its page says its dates were read in UTC.

On the select page, the projection page's rows view, the timeline and the location page, a bare `from` or `to`
is a day in the zone the `zone` parameter names, an IANA name shown beside the dates and editable. A
request without one reads in the zone the daemon is configured with (`zone`, `ANAM_ZONE`; UTC when that is
`Local` with no name to send). The page's script fills the field with the browser's own zone while nothing on
the page has been read in a zone yet, the way the entry form fills a fact's zone, and the form sends it from
then on. An unknown zone, and `Local`, are a 400 that names the zone. The `as-of` on the projection page
stays UTC: it is an anam-time, not a day.

The timeline and the select page hand a range to each other as links: "select in this range", on a
zoomed timeline only, opens `/select` with the window as `from`/`to`, the type filter and the zone, and
"show on timeline", on the select page, opens `/` at the filter's `from`/`to`, its types and the zone, with
`sel` when the page was opened at a saved selection. A bound is written as a date when it is midnight in the
zone and as an instant otherwise, so a Sydney day stays a Sydney day in both directions.

A program a person edits past what the chips can express (a join, a recursion, an `attr`) still runs,
as text. `selection.Parse` reads a program back to criteria only when the builder could have written
it, and says so when it could not.

## Saved selections

A selection is handed on as its program, and also as a short code. "Save this
selection" on the select page, or `POST /v1/selections` with `{"program": "..."}`, files the program
as a row of kind `selection`, addressed by the SHA-256 of its canonical text, and answers its
code, written `sel:<code>`. The canonical text is what `selection.Parse` then `Program` write for a
program within the chip grammar (with the zone line it names, if any), so the same selection written in
another layout or rule order has the same code; any other program is saved as its text, trimmed.

The code is the first 8 hex digits of the hash. At save, if another saved selection shares those 8,
the new one's code lengthens to the shortest prefix of its hash that no other saved selection shares,
and that lengthened code is filed as a second small row, kind `selection-code`, since which of
the two came first is not something the hashes can say. The first one keeps its 8 digits. A code is
written once and never rewritten, so it never changes meaning. Saving the same program again writes
nothing and answers the same code.

A code names the program, not its results. As the record grows the same code may select more; a
receipt is what pins the results at a watermark.

| Where | What it takes |
|---|---|
| `GET /select?sel=<code>` | the program, with the chips filled when it is within the grammar |
| `GET /?sel=<code>` | the timeline drawing only the facets the program selects in the window; `selmode=highlight` draws every facet with the selection's picked out |
| `GET /link?sel=<code>` | the link form, its basket filled from the program as `program=` fills it |
| `GET /v1/selections/{code}` | `{code, hash, url, program}`, for any role |
| anam-mcp | `save_selection` and `open_selection` |

The timeline runs the program through `read.Evaluate`, as the select page does, over the canonical's
facets whose fact time lies in the window, bounded by `max_read_bytes`. Over the bound it says so and
asks for a narrower window, which runs the program over less. Its header reads "selection
sel:<code>, N facets". A code nothing was saved under is a 404 page that says so, never an empty
timeline. A code may be given with or without `sel:`; 8 digits name the selection first saved under
them, and a longer code names the one it was lengthened to, or the one saved selection whose hash it
starts.

## Where each thing lives

| Thing | Where |
|---|---|
| The bundled rules | `internal/query/rules/anam.dl`; earlier texts a receipt may name, `internal/query/rules/history/` |
| The builtins | `internal/query/builtin.go` |
| The evaluator, the base relations, `ReadsAttr` | `internal/query` |
| The query read and its receipt | `internal/read/query.go`, `internal/serve/query.go` |
| The routes | `internal/daemon/query.go`, described in `GET /v1/openapi.yaml` |
| The selection builder | `internal/selection` |
| Saved selections and their codes | `internal/selection/saved.go`; the pages in `internal/inspect/selection_saved.go`; the routes in `internal/daemon/selections.go` |
| This page's tests | `internal/query/docs_test.go` |
