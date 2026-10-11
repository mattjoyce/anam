---
type: invariant
title: A batch is at most 49 envelopes, admitted whole or not at all
description: The gate's batch is one transaction and one anam-time instant; the daemon refuses more than 49 with 400 and the importer base cuts at 49.
timestamp: 2026-10-11T00:00:00Z
resource: internal/gate/gate.go#MaxBatch
anchor: "const MaxBatch = 49"
basis: code
tier: detail
enforced_at: internal/gate/gate.go#MaxBatch
enforced_anchor: "const MaxBatch = 49"
applies_to:
  - /components/gate.md
  - /components/daemon.md
  - /components/importer-base.md
  - /data-structures/batch-submission.md
tested_by:
  - internal/gate/batch_test.go#TestCommitBatch_RefusesOversizedBatches
  - internal/gate/batch_test.go#TestCommitBatch_WritesNothingWhenAnEnvelopeIsBad
---

docs/IMPORTERS.md §8.4. The number is inherited from the previous substrate's transaction limit and kept so that one batch is one change in Anam's knowledge. The ledger sets no limit of its own; the size is the gate's policy. All-or-nothing follows from the transaction: any refusal rolls the whole batch back.
