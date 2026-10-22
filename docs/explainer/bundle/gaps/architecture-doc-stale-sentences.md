---
type: gap
title: Sentences in ARCHITECTURE.md the code has moved past
description: Zero-padded sort keys, RFC3339Nano times, only init and ingest open for writing, retract reads the full fold, commit returns admitted or rejected, and a ledger busy lock as a 503; each is one sentence behind a later card.
timestamp: 2026-10-14T00:00:00Z
looked_at:
  - docs/ARCHITECTURE.md
  - internal/clock/clock.go
  - internal/store/ledger/schema.go
  - internal/store/ledger/ledger.sql
  - internal/app/app.go
  - internal/daemon/envelopes.go
---

In order: §12 says every integer in a sort key is zero-padded (INTEGER columns since the SQLite move); §8 says times are RFC3339Nano (`clock.Layout` refuses it because it elides trailing zeros); §8 says only init and ingest write (there has been no `ingest` command since card 119; init, the daemon and `retract --yes` write); §7 says retract reads the full fold (it composes from the last run, card 65); §6 says commit yields admitted or rejected (four outcomes: admitted, superseded, no-op, tombstoned; refusal is an error); and `ledger.ErrBusy`, a lock held by another process past 3 s, is a 500 on `POST /v1/envelopes`, not the 503 IMPORTERS §8.4 implies for every wait-and-resend case. None changes the design; each would mislead a reader who trusted the sentence.
