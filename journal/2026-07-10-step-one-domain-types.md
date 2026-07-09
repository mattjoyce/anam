---
id: 202607100014
type: journal
title: "Step one — the domain types, and a trap in the standard library"
created: 2026-07-10
status: evergreen
tags: [anam, journal, go, facet, clock, config]
project: anam
---

# Step one — the domain types, and a trap in the standard library

The second session. [[2026-07-10-genesis|Genesis]] ended with a complete spec and no Go beyond the
skeleton; this one built build-order step 1 — `facet`, `clock`, `config` — and committed it green.

## The arc

1. **Checked the ground before building on it.** Go 1.22.2, all four lint tools present, DynamoDB Local
   on unraid answering with its characteristic `400`, and
   [[reference-implementation-mint-cli|`mint-cli`]] where the note said it was. Two things were *not* as
   documented — see below.

2. **Found the trap.** The spec's §8 rests everything on "fixed-width RFC3339Nano, so lexical order
   equals chronological order." The natural Go transcription of that sentence,
   `t.Format(time.RFC3339Nano)`, **breaks it**: the layout's `9`s elide trailing zeros, so
   `…06:30:00Z` sorts *after* `…06:30:00.5Z`. Every DynamoDB `BETWEEN` over the timeline would have
   returned a subtly wrong window, quietly. Written up as
   [[fixed-width-timestamps-lexical-order]]. Catching this before the store layer existed was luck
   bought by reading §8 twice.

3. **Reconciled verbatim with idempotent.** These pull against each other: payloads are held
   [[verbatim-payload-metadata-separation|exactly as the source emitted them]], yet `commit` must detect
   an unchanged row by hash — and a Go map serialises its keys in a different order every run. The
   resolution is that *"verbatim" is a claim about content, not bytes*:
   [[canonicalisation-fixes-serialisation-not-values]]. The sharp edge is numbers, where `encoding/json`
   will happily round `9007199254740993` and rewrite `1.50` unless held to `UseNumber()`.

4. **Made §12 executable.** The do-not-break list stops being prose and becomes tests — including a
   reflection guard that fails the build if a `Status` field ever appears on `Facet`, because that
   invariant is about the type's *shape* and nothing else can enforce it. Then each guard was
   mutation-checked: break the invariant, watch the right test fail, restore.
   [[invariants-as-structural-tests]].

5. **Committed.** `scripts/check.sh` green — gofmt, goimports, vet, golangci-lint, gosec, `-race`
   `-shuffle=on`. Coverage 93.8%.

6. **Then ran the audit we had skipped.** `AGENTS.md` requires the
   [[guiding-lens-hickey-armstrong|Hickey × Armstrong]] pass on any non-trivial change, and we had
   used the *vocabulary* — "complect", "error kernel" — without ever pointing the lens. Running it
   properly found four things, each confirmed by executing it rather than reasoning about it. See
   below.

## Two corrections to the spec

Both recorded in [[source-data-locations]]:

- **`MVP.md` §8 points at the wrong `healthdata.db`.** The `/mnt` copy holds a single month and a single
  year period — it cannot demonstrate a timeline. The real database is
  `/home/matt/admin/data/healthdata.db`, with daily rows across March–June 2026 and monthly back to
  2024. A longitudinal record is the whole point of [[the-inversion]]; the demo needs the real one.
- **The BP CSV is gone.** Sent over LocalSend last session, never persisted, nowhere on disk. It must be
  re-sent and committed to `testdata/`. The `bp_*` metrics inside `healthdata.db` are *not* a
  substitute — they are period summaries, a different fact at a different grain.

## The audit — what the lens found

**Armstrong, and it was a live bug.** `config.Load` silently ignored an unrecognised key. A typo'd
`ddb_endpont` returned no error and left `DDBEndpoint` at its default, so Anam would have talked to
`localhost` while the operator believed it was talking to unraid — the failure you never notice. Config
is external input; it now fails fast on any unknown key.

**Hickey, and it was our own blindspot.** We wrote a reflection test guarding the *shape* of `Facet` (no
`Status` field may appear) and took that for having made §12 executable. But shape guards say nothing
about *values*. A `Facet` with an invalid `Type`, an empty `BirthStatus`, and a `Descriptor.ContentHash`
that disagreed with its own `Payload` compiled and passed every test — there is no constructor and no
`Validate()`. `Envelope` has one; the far more load-bearing `Facet` does not.

**`ContentHash` accepted any bytes.** Hashing a raw, key-unsorted payload compiled and returned a
plausible, wrong digest; the "these bytes are canonical" guarantee lived only in a doc comment. Fixed
with a `Canonical` type — deliberately a **struct wrapping an unexported field**, not a named `[]byte`,
because Go's assignability rules would let a raw `[]byte` pass wherever a named byte-slice type is
expected and the misuse would still compile. It is now a compile error. This is the cheap version of
[[canonicalisation-fixes-serialisation-not-values]]: make the invariant travel with the value.

**The two lenses disagreed on the cure, and that is the useful part.** Hickey's answer to a
badly-formed `Facet` is a smart constructor and unrepresentable illegal states. Armstrong's instinct
runs the other way — don't scatter defensive validation through the interior, put one check at the
single ingress and let everything inside trust it. Averaging them would give a `Validate()` sprinkled at
every call site, which is the worst of both. Holding the tension gives the right answer: **the gate is
the only thing that constructs a `Facet`**, and types carry the guarantees inward so the interior never
re-checks. `Canonical` is exactly that move — Hickey's structure serving Armstrong's isolation.

Both new guards were mutation-checked, per [[invariants-as-structural-tests]].

## Config discovery, settled

Config now resolves `--config` → `$ANAM_CONFIG` → `<user config dir>/anam/config.yaml`, with the
asymmetry that an explicitly-named missing file errors while a discovered missing file falls back to
defaults ([[config-explicit-fails-discovered-defaults]]). `person_id` lost its default, because it is
the ledger's partition key and a wrong one succeeds into the wrong partition rather than failing
([[partition-key-has-no-default]]).

Mutating the code to check a comment paid off again: the claim that `BindEnv` must follow
`SetEnvPrefix` turned out to be true but *not* for the reason written down — `AutomaticEnv` masks the
wrong order entirely. The comment now says what is actually true, and warns against relying on the mask.

`gosec` flagged `os.Stat` on an operator-supplied path (G703, taint). `filepath.Clean` satisfied it
honestly, so no `#nosec` was needed.

## The ladder, planned under Brooks × Beck

Called the second lens of the session — `Brooks × Beck` (*triage × step*), the pair whose subject is
literally "how we approach the work" — on the remaining build order, as a *Before* pass.

It reordered the MVP. `mint` sat before `fold` in §2's chain, but a checkpoint is a **cache of the
fold**: it bounds replay, and there is no replay cost to bound yet. Meanwhile `anam timeline` — the
thesis, the one claim Anam exists to make — was rung six. Timeline moved to four, mint to eight.

The synthesis is [[brooks-below-the-gate-beck-above-it]]: the error kernel already tells you where each
lens applies. Depth-first below the gate, where corruption is unrecoverable; smallest-step above it,
where everything is disposable and rebuildable.

Brooks's triage also named what we had not admitted: [[substrate-is-accidental|DynamoDB is accidental
complexity]]. Fixed-width timestamps, single-table modelling, GSIs, the marshalling envelope — none of it
is the problem, all of it is the substrate. It stays, for good reasons, but `scripts/check.sh` now
enforces that only `internal/store` may import the AWS SDK. The check was verified to fire before the
store exists.

**And rung one turned out not to be code.** The riskiest assumption in the project — that DynamoDB Local
supports the atomic conditional `TransactWriteItems` the crash-clean property rests on — was retired
with five minutes of `curl` against a scratch table, since deleted. Transactions, conditional rejection,
`BETWEEN` over our fixed-width keys, GSI2, and opaque Binary payloads all behave. Brooks's "plan to throw
one away" and Beck's "smallest step that yields information" are the same instruction.

## Where we are

`internal/{facet,clock,config}` committed on `master`. The three packages hold no I/O and no DynamoDB;
they are the values everything above will be written in terms of.

Next is **R2** on the ladder (`MVP.md` §2): the `store.DDB` layer against the live endpoint —
single-table `CreateTable` with GSI1/GSI2, `toItem`/`fromItem` per domain value with the `kind`
discriminator [[verbatim-payload-metadata-separation|quarantined to the store package]], payload as an
opaque Binary attribute, and `ConditionalCheckFailed` mapped to a typed conflict. Every one of those
mechanisms is now known to work — the spike proved them before we commit to them.

Then **R3**, the gate, which is where the error kernel begins and where
[[brooks-below-the-gate-beck-above-it|the step size changes]].

## Open threads

Carried from genesis:

- Whether `identity_key` lives in `meta` or derives from `source_ref` ([[facet-definition-format]]).

Deferred out of the audit — all three land with the gate in step 3, because the gate's shape decides
theirs:

- **`Facet` has no constructor.** Illegal facets are representable. The fix is *not* a `Validate()`
  called everywhere; it is `facet.New(envelope, resolution, clock)` living at the gate, as the only way
  to build one, computing `Descriptor.ContentHash` from the payload rather than accepting it. Writing it
  now would be guessing at a signature `resolve`/`commit` will dictate.
- **`Provenance` braids two concerns.** `Source`/`Injector`/`SourceRef` describe capture;
  `Method`/`Model`/`Watermark` describe derivation. Nothing stops a captured facet carrying a `Model`.
  A nested optional `Derivation` would make the illegal state unrepresentable.
- **`Status` and `RecordEventKind` share five of six string values.** `Status(event.Kind)` is a legal
  conversion that yields `Status("published")` — an invalid status the fold would carry silently. The
  fold must map kinds to statuses explicitly, never convert. Guard it when the fold lands.

Environmental:

- `viper` v1.21 raised the module's Go floor from 1.22 to 1.23.0. The local toolchain is 1.22.2, so
  `GOTOOLCHAIN=auto` now downloads 1.23.0 on every build. It works; it is not free.
- `Confidence` is a bare `float64` on `Facet`. It has no validation and no defined range yet — the spec
  never pinned one.

Process:

- The audit was run *after* the commit, not before. `AGENTS.md` puts it on any non-trivial change; the
  natural checkpoint is the pass before each build-order commit. It found a live bug the full green
  gate — fmt, vet, lint, gosec, race, 93.8% coverage — did not. A quality gate checks that the code is
  well-made; the lens asks whether it is the right shape. They are not substitutes.
