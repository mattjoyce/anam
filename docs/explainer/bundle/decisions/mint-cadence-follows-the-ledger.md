---
type: decision
title: Mint cadence follows the ledger
description: "The daemon mints after every write that moved W, as its own write on the queue: at once after a declared ingest, and after 2 s of quiet following importer batches."
timestamp: 2026-10-11T00:00:00Z
resource: internal/daemon/ingest.go#mintOnRequest
anchor: "const mintQuiet = 2 * time.Second"
basis: code
tier: supporting
tested_by:
  - internal/daemon/envelopes_test.go#TestPostEnvelopesMints
alternatives:
  - A size, time or rate trigger (deferred in MVP.md; refused in card 61 because a mint from the last run is 60 ms on the NAS, so no trigger earns its complexity)
  - Mint inside the ingest's transaction (refused; the mint is not part of what the ingest admitted, and its failure must fail nothing)
affects:
  - /components/daemon.md
  - /components/mint.md
  - /workflows/admit-a-batch.md
---

**Context.** Card 61. Freshness comes from the ledger; a mint exists to bound replay, so it should follow the writes rather than a clock.

**Choice.** Two triggers, one mint. A declared ingest (`POST /v1/ingest`) mints before its operation finishes, so a caller waiting on it reads from checkpoints with no tail, and the result lands on the operation as `mint.error` when it failed. An importer batch that changed W requests a mint on a one-slot channel; the worker waits until no batch has arrived for 2 s and then mints once. A local `anam ingest` with the daemon stopped does not mint.

**Why.** Importers send batches of 49 in bursts; minting after each would cost a rebuild per batch for nothing.

**What it costs.** ARCHITECTURE §7 describes only the first trigger and IMPORTERS §8.4 only the second, and a mint that fails after an importer batch is reported only in the daemon log (see [mint failure after a batch is only logged](/gaps/mint-failure-after-batch-only-logged.md)).
