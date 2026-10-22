---
type: invariant
title: Append-only is enforced by the ledger's triggers, for every writer
description: Facts are added and re-statused, never modified, never deleted; triggers on every ledger table refuse UPDATE, DELETE and an INSERT over an existing key, binding a sqlite3 shell as much as Anam.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/ledger.sql#facet_no_update
anchor: "CREATE TRIGGER IF NOT EXISTS facet_no_update BEFORE UPDATE ON facet"
basis: code
tier: supporting
enforced_at: internal/store/ledger/ledger.sql#facet_no_update
enforced_anchor: "CREATE TRIGGER IF NOT EXISTS facet_no_replace BEFORE INSERT ON facet"
applies_to:
  - /components/store-ledger.md
  - /terms/ledger.md
  - /terms/supersession.md
  - /terms/tombstone.md
tested_by:
  - internal/store/ledger/appendonly_test.go#TestAppendOnly_DatabaseRefusesEveryOverwrite
  - internal/store/ledger/appendonly_test.go#TestAppendOnly_ABackupKeepsTheTriggers
---

ARCHITECTURE §12, fifth bullet, and AGENTS.md: never add a code path that drops or disables a ledger trigger. Three triggers per table: no update, no delete, and no insert over an existing key, the last because `INSERT OR REPLACE` would otherwise overwrite a row without firing the delete trigger. Two more keep the structure sound: an artifact version n needs n-1 beneath it, and an event's seq must be exactly `MAX(seq)+1`.

A trigger is not a security boundary: whoever holds the file can drop it. The threat it answers is a refactor or a hurried session, not an adversary. `Ledger.Drop` exists for the rebuild test only.
