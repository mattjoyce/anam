---
type: invariant
title: commit is idempotent and atomic
description: Artifact, anam-event and watermark land in one transaction in one file; same identity and same hash is a no-op, a different hash supersedes, and a refused batch writes nothing, so a worker may crash mid-batch and be re-run to known-good.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/write.go#write
anchor: "if h != readW {"
basis: code
tier: core
enforced_at: internal/store/ledger/write.go#write
enforced_anchor: "if h != readW {"
applies_to:
  - /components/gate.md
  - /components/store-ledger.md
  - /terms/error-kernel.md
  - /terms/idempotency.md
tested_by:
  - internal/gate/gate_test.go#TestCommit_CrashRerunIsANoOp
  - internal/gate/batch_test.go#TestCommitBatch_IsIdempotent
  - internal/store/ledger/batch_test.go#TestAdmitBatch_StaleWatermarkWritesNothing
  - internal/gate/gate_test.go#TestCommit_ConcurrentWritersBothLand
---

ARCHITECTURE §12, eleventh bullet, second half, and §8. Inside the transaction, in order: the head is the watermark the caller read, else `ErrStaleWatermark` and the gate re-reads, re-stamps anam-time and retries; this `(facet_id, version)` is not already written, else same bytes is `ErrAlreadyWritten` (our own earlier write, a no-op) and different bytes is `ErrDivergentArtifact` (a lost update, stop); the database accepts the insert, else `ErrCorrupt`. Collapsing the first two into one conflict would either retry a write that succeeded or reject a benign re-ingest, so the mapping is the idempotency, not error handling around it.

The retry loop belongs to the gate, not the store, and there is no transaction across the two files. This, plus the triggers, is the error kernel.
