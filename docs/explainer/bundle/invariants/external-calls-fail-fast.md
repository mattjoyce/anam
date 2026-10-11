---
type: invariant
title: Every external call carries a deadline and fails fast
description: Every store method takes a context with a deadline; every daemon write runs under its job's deadline; a missing ledger is an error on open, never created by reading.
timestamp: 2026-10-11T00:00:00Z
resource: internal/daemon/queue.go#work
anchor: "ctx, cancel := context.WithTimeout(base, j.deadline)"
basis: code
tier: detail
enforced_at: internal/daemon/queue.go#work
enforced_anchor: "ctx, cancel := context.WithTimeout(base, j.deadline)"
applies_to:
  - /components/daemon.md
  - /components/store-ledger.md
  - /components/client.md
  - /components/importer-base.md
tested_by:
  - internal/daemon/daemon_test.go#TestWriteDeadline
  - internal/daemon/read_test.go#TestTimeoutIsTheShorterOfTheTwo
---

ARCHITECTURE §12, last bullet, second half, and AGENTS.md. The daemon takes the shorter of its own `timeout` and the caller's `Anam-Timeout` header; the ledger's busy timeout (3 s) sits below the default call timeout (5 s) so contention surfaces as `ErrBusy`, not a bare deadline; the importer's client bounds the whole call with one deadline and caps the answer. On the importer side, a source that fails names whether a retry can help.
