---
type: data-structure
title: Config keys
description: Every key the daemon and CLI read, from config.yaml or ANAM_* env vars, with its default; an unknown key or an unknown bucket width is refused at load.
timestamp: 2026-10-11T00:00:00Z
resource: internal/config/config.go#Config
anchor: "type Config struct {"
basis: code
tier: detail
group: contract
references: []
---

| Key | Env | Default |
|---|---|---|
| `person_id` | `ANAM_PERSON_ID` | none on purpose; pinned into the ledger at init |
| `ledger_path` | `ANAM_LEDGER_PATH` | `~/.local/share/anam/ledger.db` |
| `db_path` | `ANAM_DB_PATH` | `~/.local/share/anam/tracking.db`; must differ from the ledger |
| `artifact_dir` | `ANAM_ARTIFACT_DIR` | `~/.local/share/anam/artifacts` |
| `library_dir` | `ANAM_LIBRARY_DIR` | `documents/` beside the ledger |
| `library_folder_limit` | `ANAM_LIBRARY_FOLDER_LIMIT` | 1000 files a library folder holds before a new one goes a level deeper (card 150) |
| `zone` | `ANAM_ZONE` | `Local`; the daemon refuses it for reads |
| `listen` | `ANAM_LISTEN` | `127.0.0.1:8787` |
| `token_file` | `ANAM_TOKEN_FILE` | none; required to listen off loopback; at least 32 characters |
| `importers_file` | `ANAM_IMPORTERS_FILE` | none; read once at daemon start |
| `server` | `ANAM_SERVER` | none; routes the CLI's reads, mints and receipts to the daemon |
| `backup_dir`, `backup_keep` | `ANAM_BACKUP_DIR`, `ANAM_BACKUP_KEEP` | none, 14 |
| `ingest_timeout`, `timeout` | `ANAM_INGEST_TIMEOUT`, `ANAM_TIMEOUT` | 15 m, 5 s; `ingest_timeout` bounds one long write in the daemon (index replay, backup, the mint at start) and keeps its old name |
| `max_read_bytes` | `ANAM_MAX_READ_BYTES` | 512 KiB; 0 is unbounded |
| `bucket_width` | `ANAM_BUCKET_WIDTH` | `Monthly`; `Weekly` also accepted |
| `checkpoint_budget` | `ANAM_CHECKPOINT_BUDGET` | 1 MiB; 0 keeps the fixed width |

The config file is `ANAM_CONFIG` or `<user config dir>/anam/config.yaml`; the example file is behind these keys (see [config example is stale](/gaps/config-example-is-stale.md)).
