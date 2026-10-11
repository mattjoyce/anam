---
type: decision
title: The ledger is its own file, apart from the operational store
description: Two SQLite files, no transaction across them; the derived tier catches up by replay and is behind after a crash, never wrong.
timestamp: 2026-10-11T00:00:00Z
resource: internal/catchup/catchup.go#Sync
anchor: "if err := o.PinLedger(ctx, id); err != nil {"
basis: docs
tier: supporting
tested_by:
  - internal/catchup/catchup_test.go#TestSync_RefusesACursorAheadOfTheLedger
  - internal/store/ops/derived_test.go#TestApplyReplay_MovesRowsAndCursorTogether
alternatives:
  - One database holding the ledger and its indexes, written in one transaction (refused; then deleting the index could reach a fact, and nothing would prove the derived tier derived)
  - A ledger with its own secondary indexes (refused; an index is derived, and derived things live in the operational store)
affects:
  - /components/store-ledger.md
  - /components/store-ops.md
  - /components/catchup.md
  - /terms/operational-store.md
---

**Context.** The file is the unit SQLite deletes, locks and opens read-only by.

**Choice.** The ledger holds artifacts, events and pins, nothing else, and serves two access paths on primary keys. Everything a reader wants beyond that is an index in `tracking.db`, rebuilt by replaying events since the cursor. The operational store pins the ledger's id and refuses an index of another ledger.

**Why.** The repair of the operational store is to delete its file and replay, and that repair cannot reach a fact. Deleting it and replaying is also how the rebuild test proves the derived tier derived.

**What it costs.** The index is never written in the same breath as the ledger: a crash leaves it behind, and `anam retract` leaves it behind on purpose until the daemon's next start. Catch-up has to move its cursor conditionally in the same transaction as its rows.
