---
type: component
title: Daemon
description: The one process that owns the ledger on unraid; HTTP routes behind role-checked tokens, a single write queue, catch-up and mint after each write.
timestamp: 2026-10-17T00:00:00Z
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
  - /components/library.md
  - /components/principals.md
---

## Start

`Open` runs in this order: check the [limits](/data-structures/limits.md) (`Limits.Validate`), read the operator token (`token_file`, or refuse a non-loopback listen), build the bucket width, load the [importers file](/data-structures/importers-file.md) as [principals](/components/principals.md) (refusing a principal that holds the operator's token, or two importers naming an unbundled type), open the ledger read-write and read-only, open the operational store, check its index is not ahead of the ledger, run `guardBundled` (`def.Guard` over the bundled Definitions against the ones the store last saw, then store the current set, so a Definition that would re-key the ledger's facets stops the daemon starting), load Definitions and projection templates, open a read-only ops handle for read routes, open the artifact store. `Serve` then runs the single write worker, catches up the index as its first write, and mints if behind.

## Routes

One table in `api.go` names each route with the roles whose tokens open it (see [each route names the roles it admits](/invariants/each-route-names-the-roles-it-admits.md)). Every role is a reader's for the read routes; the table below says who else.

| Route | Roles | Does |
|---|---|---|
| `POST /v1/envelopes` | importer | admit a batch through the gate; see [admit a batch](/workflows/admit-a-batch.md) |
| `POST /v1/definitions` | importer | register Definitions for its own types; see [register a Definition](/workflows/register-a-definition.md) |
| `GET /v1/definitions[/{type}[/{v}]]` | any | publish Definitions |
| `POST /v1/documents?ext=` | importer with `doc.reference` | store one document's bytes in the [library](/components/library.md) as `<sha256>.<ext>` (cards 102, 150): 201 stored, 200 already held, 400 a bad extension, 403 a token without `doc.reference`; see [hand Anam a document](/workflows/hand-anam-a-document.md) |
| `POST /v1/projections` | operator | register a projection template (card 125); a registered version is immutable and every version stays readable for receipts |
| `GET /v1/projections[/{name}[/{v}]]` | any | publish projection templates |
| `GET /v1/snapshots/{id}` | any; an importer sees its own source's | the resend report: live, absent, carried |
| `GET /v1/timeline`, `GET /v1/changes`, `GET /v1/status`, `GET /v1/checkpoints` | any | reads, bounded by `max_read_bytes` |
| `POST /v1/receipts`, `GET /v1/receipts[/{hash}[/verify]]` | any | publish and verify; publishing writes artifacts only |
| `POST /v1/checkpoints` | operator | mint on the write queue |
| `POST /v1/backups` | operator | `VACUUM INTO` both stores under `backup_dir` |
| `/` | operator | the [inspector](/components/inspect.md) |

`/healthz` is outside the table and needs no token. A missing or unknown token is 401; a token whose role the route does not admit is 403 naming the principal.

Every unsafe request passes `browserGuard` first (card 108): a foreign `Origin`, a `Sec-Fetch-Site` other than `same-origin` or `none`, or on a tokenless daemon a `Host` that is not a loopback name, is 403 before the token is read and before any work is queued. A cached basic-auth token is exactly the credential a page on another site would spend, so the guard runs ahead of `admit`.

The daemon reads no source itself (card 119): `POST /v1/ingest`, `GET /v1/operations/{id}` and the imports file are gone. `GET /v1/status` lists each importer's source with `last_scan_at`, which now means when its last batch arrived, from a `sources` view in `minting.go` that is held in memory and forgotten on restart.

## The write queue

Every write is one job on one channel, run by one goroutine under the job's deadline. An operator write waits `accept_wait` (250 ms) to be accepted; an importer's batch waits `importer_wait` (5 s) and gets a 503 with `Retry-After` (`retry_after`, 15 s) after that. The wait spends the request's budget, the shorter of `timeout` and the caller's `Anam-Timeout`, so a request that runs out of it before the queue takes it is a 503 and its write never starts (card 110; see [queue wait spends the caller's budget](/invariants/queue-wait-spends-the-callers-budget.md)). Reads never take a turn; they use the read-only handles. Publishing writes artifacts only and takes no turn either.

## Mint cadence

An importer batch that changed W requests a mint on a one-slot channel, and `mintOnRequest` runs `mintAtHead` as its own write after `mint_quiet` (2 s) of quiet. At start `mintIfBehind` requests one when the last run is behind the head. Both live in `internal/daemon/minting.go`. See [mint cadence follows the ledger](/decisions/mint-cadence-follows-the-ledger.md).
