---
type: component
title: Client (internal/client)
description: "The Go HTTP client the CLI uses against the daemon: bearer token, a timeout header, and a 64 MiB body bound."
timestamp: 2026-10-14T00:00:00Z
resource: internal/client/client.go#New
anchor: "func New(server, tokenFile string, timeout time.Duration) (*Client, error) {"
basis: code
tier: detail
group: entry
depends_on: []
---

One method per route the CLI uses: `Timeline`, `Changes`, `Status`, `Mint`, `Checkpoints`, `Publish`, receipts and projections. Every request carries the operator token as a bearer and an `Anam-Timeout` header; the daemon takes the shorter of that and its own `timeout`. A body over 64 MiB is refused with "narrow the read". The wire types are `internal/api`, shared with the daemon, so the CLI and the daemon cannot disagree about a field name.
