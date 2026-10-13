---
type: component
title: Daemon
description: The one process that owns the ledger on unraid; HTTP routes for the operator and importers, a single write queue, catch-up and mint after each write.
timestamp: 2026-10-11T00:00:00Z
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

`Open` runs in this order: read the operator token (`token_file`, or refuse a non-loopback listen), build the bucket width, load the [importers file](/data-structures/importers-file.md) (refusing an importer that holds the operator's token, or two importers naming an unbundled type), open the ledger read-write and read-only, open the operational store, check its index is not ahead of the ledger, load Definitions, open a read-only ops handle for read routes, open the artifact store. `Serve` then runs the single write worker, catches up the index as its first write, and mints if behind.

## Routes

| Route | Auth | Does |
|---|---|---|
| `POST /v1/envelopes` | importer token | admit a batch through the gate; see [admit a batch](/workflows/admit-a-batch.md) |
| `POST /v1/definitions`, `GET /v1/definitions[/{type}[/{v}]]` | importer or operator | register and publish Definitions; see [register a Definition](/workflows/register-a-definition.md) |
| `POST /v1/projections`, `GET /v1/projections[/{name}[/{v}]]` | operator | register and publish projection templates (card 125); a registered version is immutable and every version stays readable for receipts |

Every unsafe request passes `browserGuard` first (card 108): a foreign `Origin`, a `Sec-Fetch-Site` other than `same-origin` or `none`, or on a tokenless daemon a `Host` that is not a loopback name, is 403 before the token is read and before any work is queued. A cached basic-auth token is exactly the credential a page on another site would spend, so the guard runs ahead of `authorize`.
| `GET /v1/snapshots/{id}` | importer (own) or operator | the resend report: live, absent, carried |
| `POST /v1/ingest`, `GET /v1/operations/{id}` | operator | run declared imports as an operation |
| `GET /v1/timeline`, `GET /v1/changes`, `GET /v1/status` | operator | reads, bounded by `max_read_bytes` |
| `POST /v1/receipts`, `GET /v1/receipts[/{hash}[/verify]]` | operator | publish and verify |
| `POST /v1/checkpoints` | operator | mint on the write queue |
| `POST /v1/backups` | operator | `VACUUM INTO` both stores under `backup_dir` |
| `/` | operator | the [inspector](/components/inspect.md) |

## The write queue

Every write is one job on one channel, run by one goroutine under the job's deadline. An operator write waits 250 ms to be accepted; an importer's batch waits 5 s and gets a 503 with `Retry-After` after that. Reads never take a turn; they use the read-only handles. Publishing writes artifacts only and takes no turn either.

## Mint cadence

Two triggers, both calling the same incremental mint as its own write: a declared ingest mints before its operation finishes; an importer batch that changed W requests a mint, and the request fires after 2 s of quiet. See [mint cadence follows the ledger](/decisions/mint-cadence-follows-the-ledger.md).
