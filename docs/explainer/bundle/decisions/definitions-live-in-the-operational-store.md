---
type: decision
title: Definitions live in the operational store, not the ledger
description: A Definition is how Anam interprets, not what it knew; every version is held in tracking.db, provisioned from the binary or from the importer that registers it, and a registered version is immutable.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ops/ops.go#registered_definition_no_update
anchor: "registered_definition is immutable"
basis: docs
tier: supporting
tested_by:
  - internal/store/ops/ops_test.go#TestRegisteredDefinitions_AreImmutable
  - internal/def/register_test.go#TestRegister
alternatives:
  - Admit each Definition as a fact in the ledger, as Datomic transacts schema (listed as an open decision in IMPORTERS §6, to be decided deliberately, not drifted into)
  - Hold Definitions only in the binary (how it started; importers need to register types the binary does not carry)
affects:
  - /components/def.md
  - /components/store-ops.md
  - /terms/facet-definition.md
  - /workflows/register-a-definition.md
---

**Context.** An artifact's `definition_version` must still resolve years later, and importers in other repos emit their own types.

**Choice.** Governance, not record. The `definition` table holds every bundled version the store has seen; `registered_definition` holds what importers registered, with triggers refusing update and delete. The live registry is the bundled set plus the newest registered version of each type the binary does not carry. A new version may change schema, classification or series but not `identity_key` or `fact_time`, checked when the daemon opens by `def.Guard` (`guardBundled`, which then stores the bundled set; before card 119 only an ingest ran it) and at registration by `def.Register`.

**Why.** The error kernel is smaller for it, and a lost operational store comes back from git and from each importer registering again.

**What it costs.** Receipts pin only the bundled set's hash, so a registered series rule that later changes is invisible to `receipt verify` (see [receipts do not pin registered Definitions](/gaps/receipts-do-not-pin-registered-definitions.md)).
