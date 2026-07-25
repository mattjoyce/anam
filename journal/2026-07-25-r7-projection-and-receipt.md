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
