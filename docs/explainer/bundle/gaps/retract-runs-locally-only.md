---
type: gap
title: Retract runs against the local files, never the daemon, and leaves the index behind
description: anam retract refuses a configured server and opens the files itself; there is no route or client method, so retracting the live record means stopping the daemon, and the operational index is behind until it starts again.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - internal/app/retract.go
  - internal/app/app.go
  - internal/client/client.go
  - docs/ARCHITECTURE.md
---

`retractCmd` calls `loadConfig`, which refuses when `server` is set, and `retractAll` opens the ledger read-write without calling `catchup.Sync`. ARCHITECTURE §7 still says retract reads the full fold; it plans from the last mint run and its tail (card 65). The daemon's `GET /v1/snapshots/{id}` and the CLI's plan share `read.Absent`, so they agree, but the deployed shape (one daemon owning the files on unraid) makes every retraction a stop-and-start. Whether that is intended as a safety property or is simply unbuilt is not recorded. See [retract from a snapshot](/workflows/retract-a-source.md).
