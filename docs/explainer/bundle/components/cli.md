---
type: component
title: CLI (internal/app)
description: The anam command tree; every command routes to the daemon when ANAM_SERVER is set, or opens the local files itself.
timestamp: 2026-10-11T00:00:00Z
resource: internal/app/app.go#Run
anchor: "func Run(ctx context.Context, info BuildInfo) error {"
basis: code
tier: supporting
group: entry
depends_on:
  - /components/daemon.md
  - /components/client.md
  - /components/config.md
  - /components/gate.md
  - /components/read.md
  - /components/mint.md
  - /components/serve.md
  - /components/catchup.md
  - /components/store-ledger.md
  - /components/store-ops.md
  - /components/capture-records.md
  - /components/capture-documents.md
  - /components/capture-healthdb.md
  - /components/inspect.md
---

`cmd/anam/main.go` is thin: it wires build identity and signals and hands off to `app.Run`. The cobra tree lives here.

| Command | Opens | Routed to the daemon? |
|---|---|---|
| `init` | ledger read-write (create) | no |
| `ingest` | ledger read-write, declared imports | yes (`POST /v1/ingest`) |
| `daemon` | everything, read-write | is the daemon |
| `timeline`, `changes`, `status`, `receipt`, `checkpoint` | ledger read-only | yes |
| `serve` | ledger read-only, artifacts | no, local only |
| `retract` | ledger read-only to plan, read-write with `--yes` | no, refuses to run with a server configured |
| `inspect` | ledger read-only | prints the daemon URL when one is set |

Routing is one function, `loadRoutedConfig`: with `server` set the command builds a [client](/components/client.md); without it the command opens the files. Commands that must own the files (`daemon`, `retract`, `serve`) call `loadConfig`, which refuses a configured server.

The ingest report (`ingest_report.go`) prints one line per source: envelopes, admitted, superseded, no-op, and the new W.
