---
type: invariant
title: Current status is derived by the fold, never stored on the artifact
description: A facet carries its birth status and nothing else; admitted, superseded, tombstoned, endorsed and rejected are folded from anam-events on every read.
timestamp: 2026-10-11T00:00:00Z
resource: internal/fold/fold.go#Apply
anchor: "abbreviated to facet.Status(ev.Kind): that conversion compiles"
basis: code
tier: core
enforced_at: internal/store/ledger/ledger.sql#facet
enforced_anchor: "payload            BLOB    NOT NULL,"
applies_to:
  - /components/fold.md
  - /components/store-ledger.md
  - /terms/status.md
  - /terms/facet.md
tested_by:
  - internal/facet/facet_test.go#TestFacetHasNoCurrentStatusField
  - internal/fold/fold_test.go#TestFold_DerivesEachStatus
---

ARCHITECTURE §12, sixth bullet: no place-oriented programming. The facet table has no status column and the Go struct no status field, pinned by a test that reflects over it. A tombstone is therefore an anam-event, not an artifact version, because there is nowhere on an artifact to put one.

The fold is the only derivation, and it is spelled out kind by kind rather than converting an event kind to a status, so that a new kind cannot silently become a status. The operational index persists a folded state for speed, but it is derived tier: replaying the ledger remakes it.
