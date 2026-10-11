---
type: gap
title: docs/API.md describes a conversational API that is mostly unbuilt
description: API.md (card 72) designs DescribeRecord, ListProjections, SubmitAssertion, CorrectAssertion and more as the contract for agents; the daemon's routes are the operator and importer ones in this bundle.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - docs/API.md
  - internal/daemon/daemon.go
  - internal/daemon/api.go
---

The built routes are `/v1/envelopes`, `/v1/definitions`, `/v1/snapshots/{id}`, `/v1/ingest`, `/v1/operations/{id}`, `/v1/timeline`, `/v1/changes`, `/v1/status`, `/v1/receipts`, `/v1/checkpoints`, `/v1/backups` and the inspector. API.md's common contracts (`record_position` as `{ledger_id, watermark}`, receipted reads at a W) are built in card 87; the writing side (assertions with submission ids, corrections, managed source records) is not. The `daemon --help` text also omits the snapshot route. A reader should take API.md as the direction, not the surface.
