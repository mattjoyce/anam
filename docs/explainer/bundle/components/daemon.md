---
type: component
title: Daemon
description: The one process that owns the ledger on unraid; HTTP routes for the operator and importers, a single write queue, catch-up and mint after each write.
timestamp: 2026-10-14T00:00:00Z
resource: internal/daemon/daemon.go#Open
anchor: "func Open(ctx context.Context, cfg config.Config, opts Options) (*Daemon, error) {"
basis: code
tier: supporting
group: entry
depends_on:
  - /components/gate.md
  - /components/store-ledger.md
  - /components/store-ops.md
  - /components/store-artifact.md
  - /components/catchup.md
  - /components/mint.md
  - /components/read.md
  - /components/serve.md
  - /components/def.md
  - /components/config.md
  - /components/backup.md
  - /components/inspect.md
---

## Start

`Open` runs in this order: read the operator token (`token_file`, or refuse a non-loopback listen), build the bucket width, load the [importers file](/data-structures/importers-file.md) (refusing an importer that holds the operator's token, or two importers naming an unbundled type), open the ledger read-write and read-only, open the operational store, check its index is not ahead of the ledger, run `guardBundled` (`def.Guard` over the bundled Definitions against the ones the store last saw, then store the current set, so a Definition that would re-key the ledger's facets stops the daemon starting), load Definitions and projection templates, open a read-only ops handle for read routes, open the artifact store. `Serve` then runs the single write worker, catches up the index as its first write, and mints if behind.

## Routes

| Route | Auth | Does |
|---|---|---|
| `POST /v1/envelopes` | importer token | admit a batch through the gate; see [admit a batch](/workflows/admit-a-batch.md) |
| `POST /v1/definitions`, `GET /v1/definitions[/{type}[/{v}]]` | importer or operator | register and publish Definitions; see [register a Definition](/workflows/register-a-definition.md) |
| `POST /v1/documents?path=` | importer token with `doc.reference` | store one document's bytes in the library (card 102); see [hand Anam a document](/workflows/hand-anam-a-document.md) |
| `POST /v1/projections`, `GET /v1/projections[/{name}[/{v}]]` | operator | register and publish projection templates (card 125); a registered version is immutable and every version stays readable for receipts |
| `GET /v1/snapshots/{id}` | importer (own) or operator | the resend report: live, absent, carried |
| `GET /v1/timeline`, `GET /v1/changes`, `GET /v1/status` | operator | reads, bounded by `max_read_bytes` |
| `POST /v1/receipts`, `GET /v1/receipts[/{hash}[/verify]]` | operator | publish and verify |
| `POST /v1/checkpoints` | operator | mint on the write queue |
| `POST /v1/backups` | operator | `VACUUM INTO` both stores under `backup_dir` |
| `/` | operator | the [inspector](/components/inspect.md) |

Every unsafe request passes `browserGuard` first (card 108): a foreign `Origin`, a `Sec-Fetch-Site` other than `same-origin` or `none`, or on a tokenless daemon a `Host` that is not a loopback name, is 403 before the token is read and before any work is queued. A cached basic-auth token is exactly the credential a page on another site would spend, so the guard runs ahead of `authorize`.

The daemon reads no source itself (card 119): `POST /v1/ingest`, `GET /v1/operations/{id}` and the imports file are gone. `GET /v1/status` lists each importer's source with `last_scan_at`, which now means when its last batch arrived, from a `sources` view in `minting.go` that is held in memory and forgotten on restart.

## The write queue

Every write is one job on one channel, run by one goroutine under the job's deadline. An operator write waits 250 ms to be accepted; an importer's batch waits 5 s and gets a 503 with `Retry-After` after that. Reads never take a turn; they use the read-only handles. Publishing writes artifacts only and takes no turn either.

## Mint cadence

An importer batch that changed W requests a mint on a one-slot channel, and `mintOnRequest` runs `mintAtHead` as its own write after 2 s of quiet. At start `mintIfBehind` requests one when the last run is behind the head. Both live in `internal/daemon/minting.go`. See [mint cadence follows the ledger](/decisions/mint-cadence-follows-the-ledger.md).
