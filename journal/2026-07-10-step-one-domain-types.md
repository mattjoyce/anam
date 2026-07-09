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

## Two corrections to the spec

Both recorded in [[source-data-locations]]:

- **`MVP.md` §8 points at the wrong `healthdata.db`.** The `/mnt` copy holds a single month and a single
  year period — it cannot demonstrate a timeline. The real database is
  `/home/matt/admin/data/healthdata.db`, with daily rows across March–June 2026 and monthly back to
  2024. A longitudinal record is the whole point of [[the-inversion]]; the demo needs the real one.
- **The BP CSV is gone.** Sent over LocalSend last session, never persisted, nowhere on disk. It must be
  re-sent and committed to `testdata/`. The `bp_*` metrics inside `healthdata.db` are *not* a
  substitute — they are period summaries, a different fact at a different grain.

## Where we are

`internal/{facet,clock,config}` committed on `master`. The three packages hold no I/O and no DynamoDB;
they are the values everything above will be written in terms of.

Next: the `store.DDB` layer against the live endpoint — single-table `CreateTable` with GSI1/GSI2,
`toItem`/`fromItem` per domain value with the `kind` discriminator
[[verbatim-payload-metadata-separation|quarantined to the store package]], payload as an opaque Binary
attribute, and `ConditionalCheckFailed` mapped to a typed conflict.

## Open threads

- Still unsettled from genesis: whether `identity_key` lives in `meta` or derives from `source_ref`
  ([[facet-definition-format]]).
- `viper` v1.21 raised the module's Go floor from 1.22 to 1.23.0. The local toolchain is 1.22.2, so
  `GOTOOLCHAIN=auto` now downloads 1.23.0 on every build. It works; it is not free.
- `Confidence` is a bare `float64` on `Facet`. It has no validation and no defined range yet — the spec
  never pinned one.
