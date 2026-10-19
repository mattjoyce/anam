---
type: gap
title: The query surface is decided and not built
description: Card 173 decided a Datalog query over the fold through a projection, with a receipt and a standing equality test against the snapshot; no evaluator, route, CLI command or MCP tool exists, and the card is sequenced after card 166.
timestamp: 2026-10-19T00:00:00Z
looked_at:
  - docs/ARCHITECTURE.md
  - docs/API.md
  - internal/app
  - internal/read
  - internal/fold
---

Nothing under `internal` evaluates a rule, and no route or command is named `query`. ARCHITECTURE §9 and §13 describe the read; the decision is [a read may select by payload](/decisions/a-read-may-select-by-payload-the-store-never-does.md). The evaluator and the equality test (`current/2` against `fold.Snapshot`, on the fixtures and a copy of the real ledger) touch `fold` and `read` only and may start first; introspection, `anam query`, `/v1/query` and the MCP tool land after card 166, whose `handling` block the introspection serves. Whether `fold.Fates` becomes the bundled rules is deferred until the equality test has run for a while.
