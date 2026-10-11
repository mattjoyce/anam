---
type: invariant
title: What Anam authors is authored, never authoritative
description: "Governing metadata, the ledger order, watermarks, checkpoints, receipts and provisional derived findings are Anam's own operational record: backed up, never rebuilt from a source, never the truth about the world."
timestamp: 2026-10-11T00:00:00Z
resource: docs/ARCHITECTURE.md#Invariants
anchor: "is **authored, never authoritative**; backed up, not rebuilt."
basis: docs
tier: supporting
enforced_at: not enforced
applies_to:
  - /components/store-ledger.md
  - /components/store-ops.md
  - /components/store-artifact.md
  - /terms/operational-store.md
---

ARCHITECTURE §12, second bullet. The sources are the truth about the world; the ledger is the truth about what Anam knew. The operational store is a real store, authoritative for what Anam authors, which is why each of its tables says where it comes back from: the derived tier by replay, the provisioned tier from the binary and the importers, the authored tier from a backup and nowhere else.

An earlier draft said SQLite holds nothing that cannot be rebuilt by replaying the ledger; that demoted a real database to a cache and was corrected. Nothing enforces the tier annotations; they are comments in the schema, and the snapshot tables add a fourth word, disposable.
