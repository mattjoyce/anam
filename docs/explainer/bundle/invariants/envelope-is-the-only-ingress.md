---
type: invariant
title: The Envelope is the only ingress
description: Capture workers and importers share nothing with the ledger except through an envelope handed to the gate; a capture worker may never import the store, and scripts/check.sh fails the build if one does.
timestamp: 2026-10-11T00:00:00Z
resource: scripts/check.sh#capture_store
anchor: "FAILED: a capture worker imports Anam's store:"
basis: code
tier: supporting
enforced_at: scripts/check.sh#capture_store
enforced_anchor: "FAILED: a capture worker imports Anam's store:"
applies_to:
  - /components/gate.md
  - /components/capture-records.md
  - /components/capture-documents.md
  - /components/capture-healthdb.md
  - /components/importer-base.md
  - /terms/envelope.md
---

ARCHITECTURE §12, last bullet, first half, and AGENTS.md (share-nothing). Inside the repo it is a build gate: `go list` over every package's imports, and any `internal/capture` package importing `internal/store` fails `check.sh`. Outside the repo it is the HTTP contract: an importer reaches the record only through `POST /v1/envelopes`, and its token cannot read it.
