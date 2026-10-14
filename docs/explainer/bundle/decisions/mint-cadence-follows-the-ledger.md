---
type: decision
title: Mint cadence follows the ledger
description: "The daemon mints after importer batches that moved W, once 2 s pass with no batch, as its own write on the queue; at start it mints if the last run is behind the head."
timestamp: 2026-10-14T00:00:00Z
resource: internal/daemon/minting.go#mintOnRequest
anchor: "const mintQuiet = 2 * time.Second"
basis: code
tier: supporting
tested_by:
  - internal/daemon/envelopes_test.go#TestPostEnvelopesMints
alternatives:
  - A size, time or rate trigger (deferred in MVP.md; refused in card 61 because a mint from the last run is 60 ms on the NAS, so no trigger earns its complexity)
  - Mint inside the batch's transaction (refused; the mint is not part of what the batch admitted, and its failure must fail nothing)
  - Mint before answering a declared ingest, so a caller waiting on it read from checkpoints with no tail (built, and gone with `POST /v1/ingest` in card 119)
affects:
  - /components/daemon.md
  - /components/mint.md
  - /workflows/admit-a-batch.md
---

**Context.** Card 61. Freshness comes from the ledger; a mint exists to bound replay, so it should follow the writes rather than a clock.

**Choice.** One trigger, one mint. An importer batch that changed W requests a mint on a one-slot channel; `mintOnRequest` waits until no batch has arrived for 2 s and then `mintAtHead` mints once, as its own write. At start, `mintIfBehind` requests a mint when the last run is behind the head, so a batch admitted just before a shutdown is minted. The operator can also mint by hand, `anam checkpoint` (`POST /v1/checkpoints`).

**Why.** Importers send batches of 49 in bursts; minting after each would cost a rebuild per batch for nothing.

**What it costs.** A mint that fails after an importer batch is reported only in the daemon log (see [mint failure after a batch is only logged](/gaps/mint-failure-after-batch-only-logged.md)).
