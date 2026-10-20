---
type: component
title: Client (internal/client)
description: "The Go HTTP client the CLI uses against the daemon: bearer token, a timeout header, a network margin, and a bound on the answer it reads."
timestamp: 2026-10-17T00:00:00Z
resource: internal/client/client.go#New
anchor: "func New(server, tokenFile string, timeout, margin time.Duration, maxBody int) (*Client, error) {"
basis: code
tier: detail
group: entry
depends_on: []
---

One method per route the CLI uses: `Timeline`, `Changes`, `Status`, `Mint`, `Checkpoints`, `Publish`, receipts and projections. `Receipt` reads every page of a receipt and joins them, so it answers the whole value as it always has; `ReceiptPage` reads one page by its cursor (card 92). Every request carries the `token_file` token as a bearer (the operator's, unless the file holds another role's) and an `Anam-Timeout` header; the daemon takes the shorter of that and its own `timeout`, and the client itself waits that plus `network_margin` (10 s by default). An answer over `max_response_bytes` (64 MiB by default) is refused with "narrow the read". Both numbers reach `New` from the loaded config; the `Anam-Timeout` header name is a constant because both ends of the wire must spell it alike (card 151). The wire types are `internal/api`, shared with the daemon, so the CLI and the daemon cannot disagree about a field name.
