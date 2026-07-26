---
type: journal
date: 2026-07-26
session: 7
title: "Two clocks — Fact Time and Anam Time, and the two that were never real"
tags: [anam, journal, vocabulary, bitemporal, fact-time, anam-time, rename]
---

# Two clocks — Fact Time and Anam Time, and the two that were never real

Anam had four times. It now has two, and the two it kept are named after the only two things that can
assert them: the world, and Anam.

| Time | Lives on | Asserted by |
|---|---|---|
| **Fact Time** | the Facet | the source — when it was true in the world |
| **Anam Time** | the anam-event | Anam — when its knowledge changed |

`event_time` → `fact_time`. `record_time` → `anam_time`. `record-event` → `anam-event`. `recorded_time`
and `ingested_time` are gone.

## The rename was the small half

Seventy-three files, mechanical, and the compiler found every miss. What mattered was the audit that
came first, because two of the four times did not survive it.

**`recorded_time`** was declared in the Definition DSL as an optional second payload clock — "when a
person or system documented this" — defaulting to `event_time` when a source had none. Not one of the
five bundled Definitions declared it. All five took the default. So it was a byte-for-byte duplicate
of `fact_time` across 893 rows, *and it was in the canonical entry*, which means it was in the hash: a
column that could never disagree with its neighbour, costing an attribute on every artifact and a field
in the equality oracle for nothing.

**`ingested_time`** was worse in an interesting way. It had exactly one reader — the inspector's facet
page — and its value was the anam-time of that artifact's `admitted` event, copied onto the Facet at
capture. A second home for an answer the ledger already held, which is the shape of every drift bug
Anam's design is built to make impossible. The facet page now goes to the ledger and reads the
anam-time off the event that admitted the version being displayed. One fact, one home.

Both deletions were verified before they were made, not argued for and then assumed:

```
$ rg 'recorded_time' internal/def/manifests/*.json     # nothing
$ rg -n 'IngestedTime' --type go | grep -v _test        # one read site
```

That check is the whole method. The claim "recorded time defaults to fact time" was written in a doc
comment months ago and had been true ever since — but "is defaulted" and "is *always* defaulted" are
different claims, and only the second justifies deleting the field.

## What the rename bought structurally

Before, a Facet carried three times and an anam-event carried one, which meant a Facet could in
principle answer "when did Anam learn this?" — and did, wrongly, via `ingested_time`. After, the split
is enforced by absence:

> A **Facet carries one clock and it is the world's**. An **anam-event carries one clock and it is
> Anam's**. Neither can borrow the other's, because neither has a field for it.

That is the same move as deriving `Status` from the anam-events rather than storing it: make the wrong
answer unrepresentable rather than merely discouraged.

## `birth_status` went the same way, for the same reason

Matt: *"birth_status, drop, no idea where it came from."* It came from Anam's own R0, not from Arch —
zero mentions across `/mnt/Projects/Arch/docs`. Its stated purpose was doctrinal: *"the artifact stores
only immutable `birth_status`; current status is a fold."* A constrained field, existing to prove there
was nowhere to write an unconstrained one.

Nothing read it. Every reference was marshal, unmarshal, render, or validate — no code branched on it.
And `new.go` contained the proof that it was redundant:

```go
case (p.BirthStatus == BornProposed) != (p.Derivation != nil):
    return fmt.Errorf("...derivation and birth status disagree...")
```

A rule enforcing that two fields always agree is a rule saying one of them is spare. `birth_status` was
`Derivation != nil` written a second way, with a validator to keep the copies honest.

Deleting it made the invariant **stronger**. The structural test had an exemption —
`if name == "BirthStatus" { continue }` — carved out of "no Facet field may contain the word Status".
That exemption is now gone, and the test admits none. *There is no status field* beats *there is a
status field, but a constrained one*.

It also exposed something. `proposed` is a *birth* status but not a foldable `Status` — the five
statuses and five event kinds are `admitted / superseded / tombstoned / endorsed / rejected`, and none
of them is `proposed`. So a facet born proposed has no event that folds to proposed; it would read as
something else immediately. §11's isolation of agent findings was never implemented — `birth_status`
merely looked like the mechanism. Removing it doesn't remove the feature; it stops the field pretending
the feature exists.

A second, unplanned gain: deleting *event time* freed the word **event**. It used to be ambiguous —
a fact had an event time, and Anam had record-events. Now "event" means an anam-event and nothing else,
so `facet.EventAdmitted` reads correctly for the first time.

## The bit that would have rotted silently

The ops SQLite schema renamed `record_event_index` → `anam_event_index`. Every table there is created
with `CREATE TABLE IF NOT EXISTS`, and the schema-version pin was still at 2 — so opening an existing
`tracking.db` would have created an empty `anam_event_index`, left the populated `record_event_index`
orphaned beside it, passed the version check, and reported success. `changes_since` would have gone
quiet rather than failed.

Bumped to 3. The mismatch is now an error that names the fix.

Worth stating as a rule, because it generalises past this session: **`IF NOT EXISTS` plus a renamed
object is a silent data-loss path.** The idempotence that makes `init` safe to re-run is exactly what
stops it noticing that the thing it is creating is not the thing that was there.

## Deciding what a projection is

Alongside the rename, the vocabulary got separated onto two axes that had been tangled into one.

**Governance — who may see it.** *Canonical* (everything live at a watermark, ungoverned, hashed) →
*Projection* (a named, versioned, bundled pruning; publishable and receipted) → *View* (an ad-hoc
narrowing with `--from/--to/--type`, for a person looking at their own record; not named, not
published).

**Shape — what a read returns.** *Log* (every surviving facet — what exists) versus *Snapshot* (one
facet per series, the latest at or before a fact-time — what does not exist yet).

And the two clocks pick the point: **Anam Time chooses which canonical; Fact Time chooses what within
it.**

I argued shape should be a read-time flag, on the grounds that a snapshot always shows *less* than the
log it came from and so can never leak. Matt overruled it, correctly: a read-time shape flag is the
consumer **defining** a projection rather than selecting one, and "a consumer selects a projection; it
never defines one" is load-bearing. Shape goes in the template.

The argument I had missed is that it also needs no new machinery. A template already carries a
fact-time `to` bound. A snapshot template with no `to` means "latest as of now"; with one, "latest at
or before then". The coordinate was already there.

Still open, and the crux: **`series_key`**. "The last facet of the filtered types" works for
`health.blood-pressure` (234 readings of one thing) and breaks for `health.metric-group` (299 facets
spanning activity, calories, stress, across day/week/month/year periods — "the last one" is
arbitrary). Grouping by `identity_key` fails the other way: for BP each reading *is* its own identity,
so latest-per-identity returns all 234 again. The grouping key is a third thing, and it is latent in
the identity key already — `(group, period_type)` is the series, `period_start` is the point within it.

Definition-owned, like identity. *Identity says which row this is; series says which thing it is a
repeated measurement of.*

One structural consequence, not yet resolved: this would be the first projection knob whose verdict
depends on **other entries**. The existing four judge each entry alone, which is why
`projection.Explain` can report a property ("type not in the allow-list"). Latest-in-series reports a
*relation* ("a later reading exists"). Pleasingly, that relation already has a rendering — Anam shows
`displaced by v2` on the Anam-Time axis; this is the same idea on the Fact-Time axis.

## Naming near-misses, avoided on purpose

`--as-of` means anam-time, always and unprefixed, matching Datomic and SQL:2011. The fact-time
coordinate must therefore *not* be `--as-at`: one letter apart, opposite clocks. The old architecture
text justified the unprefixed `as-of` by saying a `record-as-of` would collide with *recorded time* —
that justification died with the field, so the reasoning was rewritten rather than left standing on a
premise that no longer holds.

That is the aphorism trap in miniature: a sentence that stays true-sounding after the fact underneath
it is deleted.

## State

- `scripts/check.sh` green: vet, `golangci-lint` 0 issues, gosec clean, every package passing.
- Table dropped, `init` re-run, both sources re-ingested from scratch in 4m56s.
- `serve bp` → 234 of 1089 facets, `sha256:12af8b15…`; `receipt verify` returns **OK reproduced** and
  **OK intact**.
- Canonical hash moved twice over (the entry lost `recorded_time` and renamed `event_time`). No test
  pins it; the value tests assert equality relations, not literals, which is why the rename cost
  nothing there.

### The rebuild was only half a rebuild, and saying so matters

The ledger went from 893 facets to **1089**. That is not a bug and it is not a clean
rebuild-from-source proof either, so it should not be reported as one.

| type | before | after | |
|---|---|---|---|
| `health.blood-pressure` | 234 | 234 | same file, byte-identical result |
| `health.body-composition` | 103 | 103 | no new rows in the window |
| `health.heart-rate` | 177 | 200 | |
| `health.sleep` | 80 | 121 | |
| `health.metric-group` | 299 | 431 | |

`BloodPressure_11-July-2026` has not been touched since 11 July, and it re-ingested to exactly 234
facets — **that** is a real rebuild proof, on a real source, outside the test harness.
`healthdata.db` has an mtime of 20 July, two weeks after the last ingest, so it simply holds more
data now; fact-time now runs to 2026-07-16 rather than stopping around the 10th.

So: one source proved rebuild-determinism, one source proved ingest-of-new-data. Both are worth
having; conflating them would have turned a changed input into a fake invariant.

