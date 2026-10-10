---
name: anam-service
description: Call and operate the Anam service, `anam daemon`, the one process that owns Anam's ledger (Docker on unraid, http://192.168.20.4:8787). Use when asked to check Anam's status or health, run or trigger an Anam ingest, find out what an ingest did or why it failed, open the Anam inspector, call the Anam API or its /v1 endpoints, point the anam CLI at the daemon (ANAM_SERVER), use the Anam token, or deploy, restart or inspect the anam container. Not for editing Anam's code.
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
  pre-move copy; nothing on the ThinkPad writes a ledger any more. Ingest is the unraid gateway's
  `anam_ingest` job. `ANAM_TIMEOUT` is 30s in the container (a full mint takes ~10 s there).

## What the API allows today

| Route | Token | Does |
|---|---|---|
| `GET /healthz` | no | `ready` (200) or `not ready` (503). Nothing else. |
| `POST /v1/ingest` | yes | Runs every declared import. 202 with an operation. |
| `POST /v1/ingest?wait=10m` | yes | Same, but holds the response until the run ends (max `ingest_timeout`, 15m): 200 succeeded, 500 failed, 202 still going. |
| `GET /v1/operations/{id}` | yes | One run: state, per-source tallies, W before and after. 404 if unknown. |
| `GET /v1/status` | yes | Version, person, ledger id, W, each source as last seen, running and pending run ids. |
| `GET /v1/timeline` | yes | The CLI's timeline read. Query: `zone` (IANA, required; `Local` is refused), `from`, `to`, `at`, `as_of`, `type`, `projection`, `explain=true`, typed as on the CLI, and `w` (a watermark) in place of `as_of`. Whole, not paged; over `max_read_bytes` (default 512 KiB) it is 413 `result_too_large` with `too_large.narrow`, the parameters still unused. |
| `GET /v1/changes` | yes | Anam-events by anam-time: `since` (required), `until`, `zone`. Carries `index_seq` and `w`; `w` above `index_seq` means the index is behind. |
| `POST /v1/receipts` | yes | Publish a timeline read: the same query as `/v1/timeline`, which must name a `projection`. 201 with the entries, the receipt and its hash. The receipt records W and the narrowing; verify replays it. 413 over `max_read_bytes`, and then nothing is filed. |
| `GET /v1/receipts`, `/v1/receipts/{hash}`, `/v1/receipts/{hash}/verify` | yes | List, show (from artifacts alone), prove against the ledger. 404 unknown, 400 malformed hash, 409 a receipt this ledger cannot honour, 413 a receipt too large to show here (it still verifies). |
| `GET /v1/checkpoints` | yes | The checkpoint log. |
| `POST /v1/checkpoints` | yes | Mint from the last run (`?dry_run=true` derives only, `?full=true` folds the whole ledger, the repair). 503 at once while an ingest runs. |
| `POST /v1/backups` | yes | Back up the record into `backup_dir` (`/mnt/disk1/backup/anam/daily`), keeping 14. Waits behind a running ingest. The gateway's `anam_backup` asks nightly at 02:30. 503 without a `backup_dir`. |
| `GET /v1/definitions[/{type}[/{version}]]` | yes, or an importer's | The published Definitions: list, current manifest, one version. |
| `POST /v1/definitions` | an importer's | Register a Definition the importer emits. 201 new, 200 same body, 409 immutable or re-keyed. |
| `POST /v1/envelopes` | an importer's | A batch of up to 49 envelopes, admitted whole (200, per-envelope outcomes) or refused whole (422, every reason). 503 with `Retry-After` while a write runs. The operator's token gets 403. Contract: `docs/IMPORTERS.md` §8. |
| `GET /` and below | yes | The inspector (timeline, facets, events, documents). Browser: basic auth, any user name, the token as password. |

Importers' tokens live in `importers_file` (`ANAM_IMPORTERS_FILE`) as SHA-256 hashes beside each
importer's source and types; an importer's token opens only the three importer routes. There is no
API for retraction or for writing a record; those are local maintenance or design
(`docs/API.md`).

## The CLI against the daemon

Set `ANAM_SERVER` (or `--server`) and `ANAM_TOKEN_FILE`, and `status`, `version`, `timeline`,
`changes`, `receipt`, `checkpoint` and `ingest` ask the daemon, printing what they print against
local files. `inspect` prints the inspector's URL. No person or ledger path is needed.

```bash
export ANAM_SERVER=http://192.168.20.4:8787 ANAM_TOKEN_FILE=~/.config/anam/token ANAM_ZONE=Australia/Sydney
anam status
anam timeline --from 2026-06-01 --to 2026-06-30 --type health.blood-pressure
```

- With a server set, `init`, `serve`, `retract` and `daemon` refuse: they open local files the
  daemon owns. Unset `ANAM_SERVER` to run them as maintenance, with the daemon stopped.
- An unreachable server is an error that names it. There is no fallback to local files.
- The reader's zone travels with each read. `zone: Local` is resolved to its IANA name from
  `$TZ` or `/etc/localtime` before it is sent; if neither names it, set `ANAM_ZONE`.
- `ingest` with a server takes no source, `--imports` or `--batch`: the daemon runs its own
  imports file. It waits up to `ingest_timeout`.

Ingest takes no parameters: it runs the imports file the daemon was given
(`/etc/anam/imports.yaml` in the container), re-read at every run. To ingest one source only,
use the CLI on a local ledger, not the API.

## Calling it

```bash
T=$(cat ~/.config/anam/token); A=http://192.168.20.4:8787
curl -s $A/healthz
curl -s -H "Authorization: Bearer $T" $A/v1/status
curl -s -X POST -H "Authorization: Bearer $T" "$A/v1/ingest?wait=10m"   # blocks until done
id=$(curl -s -X POST -H "Authorization: Bearer $T" $A/v1/ingest | jq -r .operation_id)
curl -s -H "Authorization: Bearer $T" $A/v1/operations/$id              # poll
```

Use `$(cat ...)` inside the command so the token never appears in output or history.

## Reading the answers

An operation:

```json
{"operation_id":"op-2e7064296e74c166","kind":"ingest","state":"succeeded",
 "requested_at":"…Z","started_at":"…Z","finished_at":"…Z",
 "w_before":0,"w_after":13958,"changed":13958,
 "summary":"ledger changed: 13958 facets admitted or superseded",
 "sources":[{"source":"health-db","offered":5801,"admitted":5801,"superseded":0,
             "unchanged":0,"committed":5801}]}
```

- `state`: `pending`, `running`, `succeeded`, `failed`. A failed run carries `error` and keeps
  `sources` for the sources it reached: batches that committed are in the ledger.
- `committed` below `offered` means the run stopped inside that source. Re-running is always
  safe (commit is idempotent).
- `w_before`..`w_after` is the durable answer to "what landed". Operations and the per-source
  times in `/v1/status` live in memory and are lost on restart; `since` in the status says
  when the daemon started. A 404 for an id after a restart is expected: read the ledger.
- `mint` appears when W moved: the daemon minted after the ingest (`minted` of `buckets`, from
  the run at `from`, or `full`). `mint.error` is a failed mint; the ingest still succeeded.
- `summary` is `ledger unchanged: nothing new in any source` when nothing moved. The gateway's
  Discord notice keys on that string.
- At most one run is in progress and one pending. A POST during a run returns the **pending**
  run's id (the running one has already read its sources), so two POSTs can share an id.

Errors are `{"error": "…"}`: 400 a query that cannot be read (bad `wait`, a malformed date, an
unknown projection), 401 no or wrong token (with `WWW-Authenticate: Basic`), 404 unknown
operation or receipt, 409 a receipt this ledger cannot honour, 413 an answer over `max_read_bytes`
(`code: result_too_large`, `too_large: {matched, bytes, max_bytes, narrow}`), 503 while stopping, while an
ingest holds the write queue, or when the ledger cannot be read, 504 a read that ran out of time.

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
- The gateway job `anam_ingest` (unraid ductile, hourly and on `healthdata.etl.completed`)
  calls `?wait=10m`. Its failures post to Discord through `anam-ingest-failed`.
- Layout, mounts and history: vault page `unraid_admin/vault/Anam Service Container.md`.
  Code: `internal/daemon` (routes in `api.go`), wiring in `internal/app/daemon.go`.
