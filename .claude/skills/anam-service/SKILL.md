---
name: anam-service
description: Call and operate the Anam service, `anam daemon`, the one process that owns Anam's ledger (Docker on unraid, http://192.168.20.4:8787). Use when asked to check Anam's status or health, run or trigger an Anam ingest, find out what an ingest did or why it failed, open the Anam inspector, call the Anam API or its /v1 endpoints, use the Anam token, or deploy, restart or inspect the anam container. Not for reading the record's contents through the API (no read endpoints exist yet), and not for editing Anam's code.
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
- Until anam card 46, the container runs against a **scratch ledger**
  (`/mnt/disk2/appdata/anam/scratch`). The record is still the ThinkPad's
  `~/.local/share/anam/`, ingested hourly by the ThinkPad gateway. Say which ledger an answer
  came from, and check the board card before assuming the move has happened.

## What the API allows today

| Route | Token | Does |
|---|---|---|
| `GET /healthz` | no | `ready` (200) or `not ready` (503). Nothing else. |
| `POST /v1/ingest` | yes | Runs every declared import. 202 with an operation. |
| `POST /v1/ingest?wait=10m` | yes | Same, but holds the response until the run ends (max `ingest_timeout`, 15m): 200 succeeded, 500 failed, 202 still going. |
| `GET /v1/operations/{id}` | yes | One run: state, per-source tallies, W before and after. 404 if unknown. |
| `GET /v1/status` | yes | Version, person, ledger id, W, each source as last seen, running and pending run ids. |
| `GET /` and below | yes | The inspector (timeline, facets, events, documents). Browser: basic auth, any user name, the token as password. |

That is the whole surface. There is no API for timeline, snapshot, as-of, changes, receipts,
checkpoints, retraction, or writing a record. Those exist only in the CLI against local files
(card 45 routes them through the daemon), or in `docs/API.md` as design.

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
- `summary` is `ledger unchanged: nothing new in any source` when nothing moved. The gateway's
  Discord notice keys on that string.
- At most one run is in progress and one pending. A POST during a run returns the **pending**
  run's id (the running one has already read its sources), so two POSTs can share an id.

Errors are `{"error": "…"}`: 400 bad `wait`, 401 no or wrong token (with
`WWW-Authenticate: Basic`), 404 unknown operation, 503 while stopping or when the ledger cannot
be read.

## Operating the container

- Deploy committed code: `bash /mnt/Projects/unraid_admin/anam/deploy.sh` from the ThinkPad.
  It refuses a dirty tree, builds on the NAS, starts, waits for `/healthz`.
- Logs: `/mnt/Projects/unraid_admin/unraid_cmd.sh 'docker logs --tail 50 anam'`.
- Maintenance commands need the daemon stopped, then run in the container:
  `cd /mnt/user/appdata/anam && docker compose stop && docker compose run --rm anam <cmd> && docker compose up -d`.
  Never open the data directory's SQLite files from the host while `anam` runs.
- The gateway job `anam_ingest` (unraid ductile, hourly and on `healthdata.etl.completed`)
  calls `?wait=10m`. Its failures post to Discord through `anam-ingest-failed`.
- Layout, mounts and history: vault page `unraid_admin/vault/Anam Service Container.md`.
  Code: `internal/daemon` (routes in `api.go`), wiring in `internal/app/daemon.go`.
