---
name: anam-service
description: Call and operate the Anam service, `anam daemon`, the one process that owns Anam's ledger (Docker on unraid, http://192.168.20.4:8787). Use when asked to check Anam's status or health, find out what the importers last sent, open the Anam inspector, call the Anam API or its /v1 endpoints, point the anam CLI at the daemon (ANAM_SERVER), use the Anam token, or deploy, restart or inspect the anam container. Not for editing Anam's code.
---

# Anam service

`anam daemon` holds the only write handle on the ledger and the operational store. While it
runs, nothing else may open those files. Ask it over HTTP instead.

## Where it is

- Base URL: `http://192.168.20.4:8787` (unraid, container `anam`). Loopback default is
  `127.0.0.1:8787` when someone runs the daemon locally.
- Token: `~/.config/anam/token` on the ThinkPad; `/mnt/user/appdata/anam/token` on the NAS;
  `/app/secrets/anam.token` inside the ductile container. Never print it, log it, or paste it
  into a message.
- Since 2026-10-09 (anam card 46) the container holds **the record**:
  `/mnt/disk2/appdata/anam/data` (ledger, operational store, `artifacts/`, `documents/`). The
  ThinkPad's old directory is `~/.local/share/anam.moved-2026-10-09`, kept read-only as the
  pre-move copy; nothing on the ThinkPad writes a ledger any more. Every source arrives from an importer on the
  unraid gateway, posting to `/v1/envelopes`; the daemon reads no source itself (card 119). `ANAM_TIMEOUT` is 30s in the container (a full mint takes ~10 s there).

## What the API allows today

| Route | Roles | Does |
|---|---|---|
| `GET /healthz` | no | `ready` (200) or `not ready` (503). Nothing else. |
| `GET /v1/status` | reader, importer, operator | Version, person, ledger id, W, and per importer source when its last batch arrived (`last_scan_at`) and last changed the record. |
| `GET /v1/timeline` | reader, importer, operator | The CLI's timeline read. Query: `zone` (IANA, required; `Local` is refused), `from`, `to`, `at`, `as_of`, `type`, `projection`, `explain=true`, typed as on the CLI, `w` (a watermark) in place of `as_of`, and `fields` (repeated JSON pointers into the type's schema, card 138), which answers a `frame` of `columns` and `rows` in place of `entries`. Whole, not paged; over `max_read_bytes` (default 512 KiB) it is 413 `result_too_large` with `too_large.narrow`, the parameters still unused. |
| `GET /v1/changes` | reader, importer, operator | Anam-events by anam-time: `since` (required), `until`, `zone`. Carries `index_seq` and `w`; `w` above `index_seq` means the index is behind. |
| `POST /v1/receipts` | reader, importer, operator | Publish a timeline read: the same query as `/v1/timeline`, which must name a `projection`. 201 with the entries (or the `frame`, with `fields`), the receipt and its hash. The receipt records W, the narrowing and the fields; verify replays them. 413 over `max_read_bytes`, and then nothing is filed. |
| `GET /v1/receipts`, `/v1/receipts/{hash}`, `/v1/receipts/{hash}/verify` | reader, importer, operator | List, show (from artifacts alone), prove against the ledger. 404 unknown, 400 malformed hash, 409 a receipt this ledger cannot honour, 413 a receipt too large to show here (it still verifies). |
| `GET /v1/checkpoints` | reader, importer, operator | The checkpoint log. |
| `GET /v1/projections[/{name}[/{version}]]` | reader, importer, operator | The projection templates, bundled and registered: list, current, one version (card 125). |
| `POST /v1/projections` | operator | Register a projection template. |
| `POST /v1/checkpoints` | operator | Mint from the last run (`?dry_run=true` derives only, `?full=true` folds the whole ledger, the repair). 503 at once while another write holds the queue. |
| `POST /v1/backups` | operator | Back up the record into `backup_dir` (`/mnt/disk1/backup/anam/daily`), keeping 14. Waits behind a running write. The gateway's `anam_backup` asks nightly at 02:30. 503 without a `backup_dir`. |
| `GET /v1/definitions[/{type}[/{version}]]` | reader, importer, operator | The published Definitions: list, current manifest, one version. |
| `POST /v1/definitions` | importer | Register a Definition the importer emits. 201 new, 200 same body, 409 immutable or re-keyed. An `any_type` importer registers none (card 177). A Definition may carry a `handling` block, served verbatim (card 166). |
| `POST /v1/envelopes` | importer | A batch of up to 49 envelopes, admitted whole (200, per-envelope outcomes) or refused whole (422, every reason). 503 with `Retry-After` while a write runs. The operator's token gets 403. A resend-everything run labels its batches as parts of one `snapshot`. Contract: `docs/IMPORTERS.md` §8. |
| `POST /v1/documents?ext=` | importer, with `doc.reference` | Store a document's bytes in the library, named by their sha256 (cards 102, 150). 201 stored, 200 already held; never a conflict. A `doc.reference` envelope naming a sha256 the library does not hold is refused. |
| `GET /v1/snapshots/{id}` | reader, operator; an importer its own runs | A complete resend's report: parts admitted, whether it is complete, and what the source no longer holds (`absent`, judged from the snapshot's start). 404 unknown, or another importer's. |
| `GET /` and below | operator | The inspector (timeline, facets, events, documents). Browser: basic auth, any user name, the token as password. |

A POST from a browser on another site (a foreign `Origin`, or `Sec-Fetch-Site` other than
`same-origin`) is 403 before any work, whatever credential it carries (card 108); the CLI and the
importers send neither header and are unaffected. Library documents are served sandboxed, and HTML
or SVG ones download rather than render (card 106).

Any request may carry an `Anam-Timeout` header, the caller's own deadline as a Go duration
(`25s`); the daemon works to the shorter of it and its configured timeout, and a read that runs
out is 504. Every token belongs to a principal with a role (card 89, `docs/API.md` §2). The
operator's is `token_file`; the rest live in `importers_file` (`ANAM_IMPORTERS_FILE`) as SHA-256
hashes, importers under `importers` with their source and `types` (or `any_type: true`, which submits every type and registers none, card 177), readers and others under
`principals` with a name and a role. A role the route does not admit is 403, naming the
principal and the roles the route takes. There is no API for retraction; that is local maintenance (`docs/API.md`). A fact is written only
as an envelope, by an importer.

## The MCP adapter

`~/Projects/anam-mcp` (cards 72, 93, 157) is a client of this API for a model, public at
github.com/mattjoyce/anam-mcp. Its read tools are `describe_record`, `describe_type`,
`read_record` (a receipted read when it names a projection, otherwise unreceipted and it says so)
and `verify_receipt`, plus the resource `anam://receipts/{hash}`. They use `/v1/status`,
`/v1/definitions`, `/v1/timeline`, `POST /v1/receipts` and receipt show and verify.

It writes only when `ANAM_MCP_NOTE_TOKEN_FILE` names the token of the `conversation` importer
(source `conversation`, `any_type: true`, card 177). Then it offers `save_note` (the person's words
as a `note.annotation`) and `save_fact` (a facet of any type, validated by the gate, marked as
asserted by the person), both through `POST /v1/envelopes`. Unset, it writes nothing. Admission of
other sources, checkpoints, backups and the inspector stay with this skill and the CLI. Configure
it with `ANAM_SERVER`, `ANAM_TOKEN_FILE`, `ANAM_ZONE`, `ANAM_MCP_TIMEOUT` and
`ANAM_MCP_NOTE_TOKEN_FILE`.

## The CLI against the daemon

Set `ANAM_SERVER` (or `--server`) and `ANAM_TOKEN_FILE`, and `status`, `version`, `timeline`,
`changes`, `receipt` and `checkpoint` ask the daemon, printing what they print against
local files. `inspect` prints the inspector's URL. No person or ledger path is needed.

```bash
export ANAM_SERVER=http://192.168.20.4:8787 ANAM_TOKEN_FILE=~/.config/anam/token ANAM_ZONE=Australia/Sydney
anam status
anam timeline --from 2026-06-01 --to 2026-06-30 --type health.blood-pressure
```

- With a server set, `init`, `serve <projection>`, `retract` and `daemon` refuse: they open local
  files the daemon owns. Unset `ANAM_SERVER` to run them as maintenance, with the daemon stopped.
  A bare `anam serve` only lists the bundled projections and needs neither. To publish a read
  against the daemon, `POST /v1/receipts`; the CLI has no command for it yet.
- An unreachable server is an error that names it. There is no fallback to local files.
- The reader's zone travels with each read. `zone: Local` is resolved to its IANA name from
  `$TZ` or `/etc/localtime` before it is sent; if neither names it, set `ANAM_ZONE`.

## Calling it

```bash
T=$(cat ~/.config/anam/token); A=http://192.168.20.4:8787
curl -s $A/healthz
curl -s -H "Authorization: Bearer $T" $A/v1/status
```

Use `$(cat ...)` inside the command so the token never appears in output or history.

## Reading the answers

`/v1/status` sources and `since` live in memory and are lost on restart; `since` says when the
daemon started. The durable answer to "what landed" is the ledger: `GET /v1/changes`.

Errors are `{"error": "…"}`: 400 a query that cannot be read (a malformed date, an
unknown projection), 401 no or wrong token (with `WWW-Authenticate: Basic`), 403 a token whose role the route does not admit, 404 unknown
receipt, 409 a receipt this ledger cannot honour, 413 an answer over `max_read_bytes`
(`code: result_too_large`, `too_large: {matched, bytes, max_bytes, narrow}`), 503 while stopping, while a
long write holds the write queue, or when the ledger cannot be read, 504 a read that ran out of time.

## Operating the container

- Deploy committed code: `bash /mnt/Projects/unraid_admin/anam/deploy.sh` from the ThinkPad.
  It refuses a dirty tree, builds on the NAS, starts, waits for `/healthz`.
- Logs: `/mnt/Projects/unraid_admin/unraid_cmd.sh 'docker logs --tail 50 anam'`.
- Maintenance commands need the daemon stopped, then run in the container. Export the deployed
  version first: the compose file tags the image `anam:${ANAM_VERSION:-dev}`, so without it both
  `run` and `up` take the stale `anam:dev` image, and `up` restarts the daemon on old code
  (2026-10-09, card 56). The version is in `GET /v1/status`.
  `cd /mnt/user/appdata/anam && export ANAM_VERSION=<v0.N-hash> && docker compose stop && docker compose run --rm -T anam <cmd> && docker compose up -d`.
  Never open the data directory's SQLite files from the host while `anam` runs.
- Every source posts envelopes from its own importer on the unraid gateway; `records` and
  `documents` are `anam_records` and `anam_documents` since card 119, and the hourly
  `anam_ingest` job is gone with `POST /v1/ingest`.
- Layout, mounts and history: vault page `unraid_admin/vault/Anam Service Container.md`.
  Code: `internal/daemon` (the route table with each route's roles in `api.go`, principals in
  `auth.go`), wiring in `internal/app/daemon.go`.
