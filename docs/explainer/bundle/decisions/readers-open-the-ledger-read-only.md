---
type: decision
title: Commands that only read open the ledger read-only
description: Every read command and every daemon read route uses a mode=ro handle, so SQLite itself refuses a write; only init, retract --yes and the daemon open it read-write.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/ledger.go#OpenReadOnly
anchor: "return openExisting(ctx, path, personID, modeReadOnly)"
basis: code
tier: detail
tested_by:
  - internal/store/ledger/open_test.go#TestOpenReadOnly_RefusesEveryWrite
  - internal/store/ledger/open_test.go#TestOpenReadOnly_IsNotBlockedByAnOpenWrite
alternatives:
  - One read-write handle shared by every command (refused; a reader that can write is a refactor away from writing)
affects:
  - /components/store-ledger.md
  - /components/cli.md
  - /components/daemon.md
---

**Context.** The threat the triggers answer is a refactor or a hurried session, not an adversary; a read-only handle is the same answer one layer down.

**Choice.** `OpenReadOnly` opens with `mode=ro` and skips index creation; the daemon keeps one for every read route, the inspector and `/healthz`. The write handle's DSN adds WAL, synchronous FULL and an immediate transaction lock.

**Why.** A read-only open is also not blocked by an open writer, so reads proceed while a batch commits.

**What it costs.** ARCHITECTURE §8 still says only init and ingest write, which stopped being true when the daemon and retract arrived and `ingest` went in card 119; the comment on `openLedgerReadOnly` now names init, the daemon and retract (see [architecture doc has stale sentences](/gaps/architecture-doc-stale-sentences.md)). The operational store is opened read-write by local read commands, so a local `anam timeline` creates `tracking.db`.
