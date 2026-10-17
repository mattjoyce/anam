---
type: decision
title: A tombstone is an event, not an artifact version
description: Retraction appends a tombstoned anam-event against the current version and writes no artifact, because an Anam artifact has nowhere to carry a status.
timestamp: 2026-10-11T00:00:00Z
resource: internal/gate/commit.go#Retract
anchor: "Kind: facet.EventTombstoned, FacetID: facetID, AnamTime: g.clock.Now(),"
basis: docs
tier: supporting
tested_by:
  - internal/store/ledger/ledger_test.go#TestMark_TombstoneAdvancesWatermarkWithoutArtifact
  - internal/gate/gate_test.go#TestCommit_AdmitNoopSupersedeRetract
alternatives:
  - A tombstone as a status-bearing artifact version (impossible here, since status is folded out of the artifact)
  - A delete (refused everywhere; the triggers forbid it)
affects:
  - /components/gate.md
  - /components/store-ledger.md
  - /terms/tombstone.md
  - /invariants/status-is-derived-never-stored.md
---

**Context.** Anam folds status out of the artifact. A tombstone as a status-bearing artifact version has nowhere to put the status.

**Choice.** The ledger's primitive is two calls, not one with an optional artifact: `AdmitBatch` writes artifacts with their events, `Mark` writes an event alone. The status transition is the whole event. It advances W like any mutation, so a checkpoint cannot reuse a value that still lists a retracted facet.

**Why.** An optional artifact pointer would make "no artifact, admitted" representable. Keeping the tombstone an event keeps one truth per fact.

**What it costs.** Nothing visible; a read as of before the tombstone still shows the facet, and the artifact stays forever.
