---
type: change-path
title: Add a read command that works locally and over the daemon
description: A new read lives in internal/read against a read.Source, gets a daemon route on the read-only handles, a client method, and a cobra command that routes on cfg.Server.
timestamp: 2026-10-11T00:00:00Z
resource: internal/app/timeline.go#readTimeline
anchor: 'if cfg.Server != "" {'
basis: code
tier: detail
touches:
  - /components/cli.md
  - /components/client.md
  - /components/daemon.md
  - /components/read.md
  - /invariants/each-route-names-the-roles-it-admits.md
  - /decisions/reads-over-the-daemon-are-bounded.md
  - /decisions/readers-open-the-ledger-read-only.md
  - /workflows/read-the-timeline.md
---

1. **Wire types** in `internal/api`: a query with `Values()` and an `Of(url.Values)` parser, and a result struct. Both sides share them, so names cannot drift.
2. **The read** in `internal/read`, taking `read.Source` (ledger, checkpoints, artifacts, Definitions). Compose from checkpoints through `mint.ReadAt` or `ReadWindow` where it can; fall back to the fold. Keep payloads unparsed.
3. **The route** in `internal/daemon/api.go` on the read-only handles (`d.read`, `d.readOps`), as an entry in the route table that names the roles it admits (`anyRole` for a read; see [each route names the roles it admits](/invariants/each-route-names-the-roles-it-admits.md)), with `d.timeout(r)` as the deadline and `read.Fit` against `max_read_bytes` before writing. No turn on the write queue.
4. **The client method** in `internal/client`, one per route, and a command in `internal/app` that calls `loadRoutedConfig` and branches on `cfg.Server`: client when set, otherwise `readSource` with the ledger opened read-only.
5. **Tests.** A test in `internal/app` that runs the command both ways against a real ledger and ops file in `t.TempDir()` and asserts the same answer and the same refusals, as `publish_internal_test.go` does.
6. **check.sh** must stay green: no SQL outside the store, gofmt, vet, lint, gosec, race.
