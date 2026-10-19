---
type: component
title: CLI (internal/app)
description: The anam command tree; every command routes to the daemon when ANAM_SERVER is set, or opens the local files itself.
timestamp: 2026-10-14T00:00:00Z
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
  - /components/inspect.md
---

`cmd/anam/main.go` is thin: it wires build identity and signals and hands off to `app.Run`. The cobra tree lives here.

| Command | Opens | Routed to the daemon? |
|---|---|---|
| `init` | ledger read-write (create) | no |
| `daemon` | everything, read-write | is the daemon |
| `timeline`, `changes`, `status`, `receipt`, `checkpoint`, `projection` | ledger read-only | yes |
| `serve` | ledger read-only, artifacts | no, local only |
| `retract` | ledger read-only to plan, read-write with `--yes` | no, refuses to run with a server configured |
| `rename` | ledger read-only to plan, read-write with `--yes` | no, refuses to run with a server configured |
| `inspect` | ledger read-only | prints the daemon URL when one is set |

Routing is one function, `loadRoutedConfig`: with `server` set the command builds a [client](/components/client.md); without it the command opens the files. Commands that must own the files (`daemon`, `retract`, `rename`, `serve`) call `loadConfig`, which refuses a configured server.

There is no `ingest` command since card 119. Facts reach the ledger only from importers, through the daemon's `POST /v1/envelopes`; the CLI writes the ledger only to create it (`init`), to tombstone (`retract --yes`), and to carry facets into a renamed type as new versions of themselves (`rename --yes`, card 140, [workflow](/workflows/rename-a-type.md)). `openLedgerForWrite` and `registryOf` live in `init.go`.
