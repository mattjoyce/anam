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
| `timeline`, `changes`, `coverage`, `status`, `receipt`, `checkpoint`, `projection` | ledger read-only | yes |
| `query run`, `query describe` | ledger read-only | yes |
| `query check` | a real ledger read-only, to hold the bundled `current/2` equal to `fold.Snapshot` | no, refuses a configured server unless `--ledger` names a copy |
| `daemon ready` | nothing: asks `/healthz` on the configured address and exits by the answer; the container's health check | no, asks the address, not `server` |
| `move-artifacts` | the operational store read-write, to read an old artifact folder in (`--yes` writes; card 215) | no, local only |
| `version` | nothing | prints the build, and the daemon's when `server` is set |
| `hold` | nothing local: the daemon's own admission holds | yes, and it needs the daemon |
| `serve` | ledger read-only, artifacts | no, local only |
| `retract` | ledger read-only to plan, read-write with `--yes` | no, refuses to run with a server configured |
| `rename` | ledger read-only to plan, read-write with `--yes` | no, refuses to run with a server configured |
| `migrate` | the old ledger and store read-only; the new pair created and written with `--yes` | no, takes its files from flags and ignores a configured server |
| `inspect` | ledger read-only | prints the daemon URL when one is set |

Routing is one function, `loadRoutedConfig`: with `server` set the command builds a [client](/components/client.md); without it the command opens the files. Commands that must own the files (`daemon`, `retract`, `rename`, `serve`, `move-artifacts`) call `loadConfig`, which refuses a configured server.

There is no `ingest` command since card 119. Facts reach the ledger only from importers, through the daemon's `POST /v1/envelopes`; the CLI writes the ledger only to create it (`init`), to tombstone (`retract --yes`), and to carry facets into a renamed type as new versions of themselves (`rename --yes`, card 140, [workflow](/workflows/rename-a-type.md)), and to make a new ledger and carry the live facets of an old one into it (`migrate --yes`, card 200, [workflow](/workflows/migrate-to-a-fresh-ledger.md)). `openLedgerForWrite` and `registryOf` live in `init.go`.

`anam hold <source>...` puts a source's admission on hold, `anam hold --all` holds every source, `anam hold --release [<source>...]` releases them, and `anam hold` alone lists what is held.

A second binary, `cmd/anam-bench`, is the read performance bench (`internal/bench`): `gen` builds a synthetic record of a named shape through the real gate, `run` times every read scenario against it, `mint` mints it as `anam checkpoint` does, `sizes` records the facet count of each checkpoint that has none, and `compare` prints two runs' medians side by side. It is not part of `anam` and the container image does not build it.
