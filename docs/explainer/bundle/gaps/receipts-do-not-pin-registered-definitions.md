---
type: gap
title: Receipts pin the bundled Definition set only
description: definition_set_version hashes the embedded manifests; a registered Definition that decides a snapshot's closing types is not in it, so a meds-style serve against a registered series rule that later changes verifies with no note.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - internal/def/setversion.go
  - internal/read/receipt.go
  - internal/serve/serve.go
  - docs/IMPORTERS.md
---

IMPORTERS §8.7 lists this under 'Not yet'. `SetVersion` reads `manifestFS` only; `ServeDeps` passes the live registry (bundled plus registered) as the series rules. Today no registered type carries a series, so nothing is wrong yet. See [Definitions live in the operational store](/decisions/definitions-live-in-the-operational-store.md).
