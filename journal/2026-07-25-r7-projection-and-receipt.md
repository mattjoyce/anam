---
type: journal
date: 2026-07-25
session: 6
title: "R7 — the projection publishes, the receipt proves, and the retry loop disappears"
tags: [anam, journal, r7, projection, receipt, consent, as-of]
---

# R7 — the projection publishes, the receipt proves, and the retry loop disappears

Anam can now **publish**. `anam serve bp` prunes the whole-person canonical through a bundled template
and writes two content-addressed files; `anam receipt verify <hash>` re-folds the ledger at that
receipt's own watermark and says whether the promise still holds. Live over the real data: 234 of 893
facets served, `sha256:5c3bc95e…`, and both proofs green.

Run through The Algorithm at E3 — 44 criteria, 42 passed, 2 tombstoned as entailed by others.

## The thing that fell out

The rung was supposed to be additive. Instead it deleted code.

`fold.Read` had a retry loop. It existed because the read did two things in one lookup: decide which
facets are live (by folding record-events) and fetch each one (by asking the ledger for its *current*
version). Those disagree whenever a supersession lands mid-read, so the read had to notice the
disagreement and start over — `readAttempts`, `outpaced`, `ErrOutpaced`.

Building an as-of read forced the two apart. A receipt names a historical watermark, and at that
watermark a facet may be at v2 while the ledger's current is v3, so the fetch has to ask for *the
version the fold derived*. And once it asks that question, **there is no race left to retry**: an
artifact version is immutable once written, so the answer does not depend on when you ask.

`Read` became `Head()` then `ReadAt(head)`. The retry machinery went in the bin — not left as code that
can no longer fire, which would be leaving a lie.

Then the class-sweep found the same braid three more places. `catchup.Sync` had its own copy of the
retry, its own `ErrOutpaced`, and its own outpaced-detection in `advance` — all gone the same way. And
`inspect` had a latent version of the bug rather than merely the complexity: for *dead* facets it folded
to `st.Version` and then fetched the current one, so a facet tombstoned at v2 and later re-captured
would have rendered v3's content under v2's status. A row that was never true.

Two sites were correctly excluded: the gate's two `CurrentVersion` calls genuinely want what is in
force — that is what it compares content hashes against.

> Four members fixed, two non-members named. Fixing the one in front of you and not enumerating the
> class is how a one-line bug survives twenty sessions.

## Prune-only, made structural

`projection.Apply` takes a `fold.Value` and returns a `fold.Value`, rebuilt from a subset of the entries
it was handed. There is no code path that could emit an entry the canonical did not contain, and the
served serialisation is the canonical's own rather than a second one that could drift from it. A
bespoke `Served` struct would have had to re-implement the ordering and the marshalling — that is, to
reshape by accident.

The property test enumerates the *whole* knob space rather than sampling it: 4 type-lists × 4
status-lists × 3 classifications × 4 windows = 192 templates, each asserting every served entry is
deep-equal to a canonical one. The knobs are four small closed sets, so the cross-product **is** the
universal quantifier.

The consent seam (§10) is a parameter of `Apply`, so a serve path that forgets it does not compile. And
`consent.Request` has no payload field, which makes "filter on metadata only" a property of the type
rather than a rule the future policy author has to remember.

## Two proofs, kept apart

A receipt could have had one boolean. It has two, and they fail independently:

- **reproduced** — the ledger, folded at the receipt's watermark and pruned through the same template
  version, still produces those bytes.
- **intact** — the served artifact on disk still hashes to the address it is filed under.

A tampered file under a sound ledger fails the second and passes the first. Verified for real: one
character in the published file, `"137.0"` → `"138.0"`, and the output was `OK reproduced` /
`FAIL intact`, exit 1. Collapsing them would let each hide behind the other.

Two more distinctions the verifier draws rather than flattening:

- A **template version** mismatch is fatal — the template is what produced the pruning, and reporting
  "not reproduced" would blame the ledger for a change in the code.
- A **definition-set version** mismatch is a note. Definitions govern capture, not the fold, so they
  cannot change bytes already admitted. It is carried for provenance, and it gates nothing.

An artifact that is *absent* is neither pass nor fail — the ledger can still honour a receipt whose file
was deleted, and printing `FAIL intact` there would read as tampering.

## Named costs

- **The canonical hash moved**, `sha256:a3d3510f…` → `sha256:883f0dac…`, from adding `Classification`
  to `fold.Entry`. The fourth projection knob is a maximum classification and the value could not
  answer it; the precedent was already in the code, where `Status` is carried "because projections gate
  on it". Free *now* — nothing persists a canonical hash yet, R8 is unbuilt, the `checkpoint` table is
  empty, and the rebuild test computes both sides fresh. It would not have been free after R8.
- **The classification knob is exercised only by tests.** Every bundled definition says `Sensitive`, so
  no bundled template can discriminate on it over the real ledger — `bp` and `sleep` must set the
  ceiling to `Sensitive` or serve nothing. The knob is real and covered across all three levels; it
  simply has nothing to separate until a definition set has variety. Saying so, so that nobody later
  reads the live serve as evidence it was exercised.
- **`os.Root`, not a validated string join.** gosec flagged three G304s on the artifact store's file
  I/O, and `//nolint` is golangci-lint's directive, not gosec's. `internal/safepath` is the right
  precedent for a path that could be *anywhere*; the artifact store is a fixed root, so
  `os.OpenRoot` makes escape impossible at the kernel instead of arguing it away in a comment. The
  digest regex stays as defence in depth and for the better error message. The codebase's `#nosec`
  count is still **1**.
- **No SQLite receipt index**, though `config.example.yaml` anticipates one. For one person the
  content-addressed directory *is* the index, and `receipt list` scans it. Revisit when published-time
  becomes a query.
- **The ledger flake is caught.** The last entry asked for it to be captured if it recurred, and it did:
  `TestInit_RefusesMismatchedSchema`, failing at exactly its 5s deadline under the full
  `-race -shuffle=on` suite and passing in 0.52s alone. **Contention, not a race.** DynamoDB Local is
  one process on one file serving every package's CreateTable and Drop at once; a tight budget there
  does not catch a slow server, it catches a busy one and calls it a broken test. `rawDynamo`'s budget
  is now 30s and says why. The deadline still exists, so a genuine hang still fails.

## Verbatim, a third time

The MVP §9 demo detail came through the projection untouched — `"No filofopine"` on 2026-01-24 and
`"No filodopine"` the next day, both misspellings of felodipine, letter for letter, with the numbers
still source strings (`"systolic":"114.0"`). A projection includes or excludes whole facets and touches
nothing inside them, and the served file is the proof.

## Where this leaves the MVP

R7 is done. `serve`, `receipt list`, `receipt verify` join `timeline`, `changes`, `inspect` and
`ingest`. Remaining: **R8** (mint — the checkpoint machinery, whose seam has been present since R2) and
**R9** (the derived insight, admitted `proposed` — the status filter is already built and tested for it).

`scripts/check.sh`: CHECK OK — 20 packages green under `-race -count=1 -shuffle=on -vet=all`,
`golangci-lint 0 issues`, gosec silent.

## Addendum, same day: the viewer was missing, and Matt caught it

R7's criteria all passed, and the rung was still incomplete. `MVP.md` §6 asks for the *CLI timeline
viewer switching projections to demonstrate governance*; my ISA's Out-of-Scope said "a shipped UI", and
I let that swallow "the viewer switches projections", which is a different claim. Publishing worked;
*looking* did not. The only way to see what `bp` had served was to open the JSON by hand.

Matt asked "do we have a projection viewer" and then added the requirement that turned out to be the
interesting one: *select a time point, and see which events contribute to the projection, and which are
superseded.*

### Three views, because they answer different questions

- **`timeline --projection bp`** — the live lens. Folds now, prunes through the template, prints the
  rows, and puts the **served hash in the header**. That hash is the one a receipt carries, so the
  viewer and the publisher can be checked against each other by eye. Confirmed live: both read
  `sha256:5c3bc95e…`.
- **`receipt show <hash>`** — the published artifact, read from the artifact tier **alone**. No ledger,
  no fold, no re-derivation. That restraint is the point: `verify` asks whether the ledger still honours
  a receipt, and `show` asks what the artifact itself says — the question you must be able to answer
  when the ledger is gone, unreachable, or exactly the thing you doubt. Both files are checked against
  their own content addresses on the way out, so a tampered artifact is refused rather than displayed as
  genuine. The test passes a `Deps` carrying nothing but the store, so reaching for a ledger would panic
  rather than quietly pass.
- **`/projection`** in the web inspector — a template dropdown and an as-of control. Governance by
  clicking.

### `--explain`, and the two clocks

`--explain` shows every facet with its standing: `served`, `excluded` with the reason, or `superseded`.
`--as-of <record-time>` winds Anam's own clock back. Event-time asks when things happened; record-time
asks when Anam found out — and *supersession is a record-time fact*, so the second clock is the one this
question needs.

Live, over the real ledger, that reads:

```
2026-07-10T20:00:00Z   W=659  live=659  served=0    ← the BP export had not arrived
2026-07-10T21:05:00Z   W=781  live=781  served=122  ← mid-ingest
2026-07-10T22:00:00Z   W=893  live=893  served=234
```

And on a scratch ledger where a reading *was* corrected:

```
verdicts   3 served, 0 excluded, 1 superseded
06:36  v1  superseded  displaced by v2
06:36  v2  served
```

with `--as-of` before the correction reporting `0 superseded` and systolic `114.0`, against `113.0` now.

### The mistake I nearly repeated

I argued in the morning that the explainer must be the *same code path* as the filter — so `Apply` is
now built from `Explain`, and a test asserts they agree across the full cross-product of templates. Then
I wrote the "which versions are displaced" derivation **twice**: once in `internal/app/explain.go` and
once in `internal/inspect/projection.go`. Same drift, other half.

It is now `fold.Displaced`, one function, tested once. Neither viewer can give a different answer,
because neither computes one. The lesson is not "don't duplicate" — it is that the argument for sharing a
decision procedure applies to *every* half of the decision, and having just made it about one half is no
protection at all.

### Named costs

- **The gate's test timeout was a default, not a decision.** `internal/inspect` hit Go's 600s
  per-package limit under the full suite — 44s alone. Same class as the morning's `rawDynamo` flake:
  one DynamoDB Local process, every package creating and dropping its own table, `-race` on top. Two
  fixes: the six new projection-page tests were consolidated to four (table churn is the real cost
  driver), and `check.sh` now passes `-timeout 25m` with a comment saying why. A genuine hang still
  fails; it just has to actually hang.
- **The README's quick-start was stale.** It still passed `--db`/`--csv`, removed when imports became
  declared, and omitted `ANAM_PERSON_ID`, which has no default — so a new reader's first command would
  have failed on the partition key. Rewritten, and every documented flag checked against `--help`.
- **The gate caught two bugs the single-package run could not.** Under the full suite all four
  projection-page tests failed, and neither cause was contention alone:
  - **A client error and a server error were conflated.** `readForQuery` returned both "your `as-of`
    string is malformed" and "the ledger read failed", and the handler mapped everything to **400**. So
    under load the page told the caller their perfectly good request was malformed. Only a bad `as-of`
    is the caller's fault now; everything else is a 500. Worth stating plainly: this was a real defect
    that a passing local run had hidden, and the only reason it surfaced is that the gate runs the
    packages together.
  - **Four tables where one would do.** The four new tests are strictly read-only over the same seed,
    and each was creating its own table. `AGENTS.md` asks for a unique table per test *run* — nothing
    shared across runs or packages — and four CreateTables inside one package buys no isolation while
    costing real time on a DynamoDB Local every other package is also hammering. One shared fixture
    took the package from **44s to 11.3s**. The fix was to remove load, not to raise a limit; the 25m
    budget stays because the suite genuinely is that heavy, but it is no longer load-bearing here.
- **`fold.Displaced` is sequential**, one round-trip per version no longer in force, while its sibling
  `fetchLive` uses sixteen workers. The asymmetry is deliberate for now: the real ledger has zero
  displaced versions, the cost is documented at the function, and adding untested concurrency at the end
  of a session to solve a problem nobody has is worse than a correct simple loop. It becomes worth doing
  the first time a source re-exports everything in a changed format.
- **The explain view is O(displaced) ledger round-trips**, one per version no longer in force. That is
  proportional to how much the record has been *corrected*, not how large it is: the real ledger has
  zero corrections and pays nothing. If a source ever re-exports everything with a changed format, this
  is the read that notices.
