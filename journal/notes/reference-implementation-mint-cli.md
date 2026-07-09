---
id: 202607100009
type: note
title: "Reference implementation — rec-mint-cli"
created: 2026-07-10
status: evergreen
tags: [anam, note, reference, go, dynamodb]
project: anam
related: ["[[facet-definition-format]]", "[[mint-unit-whole-timeline]]", "[[person-substrate-grain]]"]
---

# Reference implementation — rec-mint-cli

> **Status (2026-07-10):** largely **superseded** for the store layer. Anam moved to an embedded SQLite
> ledger ([[substrate-is-accidental]]), so the AWS wiring, single-table `CreateTable` and
> conditional-write conflict mapping below no longer apply. What survives is the **fold-over-events**
> pattern and the **Facet Definition loader**. Kept for the record.

**What.** `/mnt/Projects/rich-episodic-context/poc/mint-cli` (`rec-mint-cli`) is a working Go prototype
of the Arch mint tier over **DynamoDB Local**. It is the closest thing to a reference implementation for
Anam's store/ingest/fold layer, and materially de-risks the build.

## What to reuse

- **AWS wiring** — `config.LoadDefaultConfig` + `WithRegion` + static fake creds
  (`NewStaticCredentialsProvider("fake","fake","")`) + `dynamodb.NewFromConfig` with
  `o.BaseEndpoint = endpoint`. DynamoDB Local doesn't authenticate; the "creds" are literally `fake`.
- **Single-table `CreateTable`** — PK (hash, S) + SK (range, S), `PayPerRequest`; swallow
  `ResourceInUseException` for idempotent `init`.
- **Conditional-write conflict mapping** — `ConditionalCheckFailedException` and
  `TransactionCanceledException` (with a `ConditionalCheckFailed` reason) → a typed `ErrConflict`.
- **Facet Definition loader** — the `meta` + JSON-Schema-subset registry (see
  [[facet-definition-format]]).
- **Fold-over-events mint** — the pattern of building canonical state by folding ordered mutation events.

## What NOT to copy

- It is **episode-keyed** (`EpisodePK(episodeID)`, `META`/`FACET#…#vN`/`EVENT#<seq>`). Anam is
  **[[person-substrate-grain|person-keyed]]** and adds GSI1 (by type) and GSI2 (changes_since).
- Its payloads are **flat string maps**; Anam's are **nested JSON** addressed by JSON Pointer.
- Its cobra layout uses package-level flag vars; Anam follows the standards — thin `main` → `app.Run(ctx)`,
  config via viper, narrow point-of-use store interfaces.

## Live endpoint

The DynamoDB Local these both target sits on unraid at `http://192.168.20.4:8000` — confirmed reachable
this session (a bare GET returns HTTP 400, which is exactly what DynamoDB Local does when alive).
