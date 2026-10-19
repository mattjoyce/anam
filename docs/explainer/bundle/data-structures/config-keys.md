---
type: data-structure
title: Config keys
description: Every key the daemon and CLI read, from config.yaml or ANAM_* env vars, with its default; an unknown key or an unknown bucket width is refused at load.
timestamp: 2026-10-19T00:00:00Z
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
| `token_file` | `ANAM_TOKEN_FILE` | none; required to listen off loopback; at least 32 characters (a floor, not a default); the operator's token, held by the daemon as a hash |
| `importers_file` | `ANAM_IMPORTERS_FILE` | none; read once at daemon start; names importers and other principals |
| `server` | `ANAM_SERVER` | none; routes the CLI's reads, mints and receipts to the daemon |
| `backup_dir`, `backup_keep` | `ANAM_BACKUP_DIR`, `ANAM_BACKUP_KEEP` | none, 14 |
| `long_write_timeout`, `timeout` | `ANAM_LONG_WRITE_TIMEOUT`, `ANAM_TIMEOUT` | 15 m, 5 s (`DefaultLongWriteTimeout`, `DefaultTimeout`); `long_write_timeout` bounds one long write in the daemon (index replay, backup, the mint at start). Its old name `ingest_timeout` (`ANAM_INGEST_TIMEOUT`) is still read, with a warning at load; the two given differently are refused (card 156) |
| `max_read_bytes` | `ANAM_MAX_READ_BYTES` | 512 KiB; 0 is unbounded |
| `bucket_width` | `ANAM_BUCKET_WIDTH` | `Monthly`; `Weekly` also accepted |
| `checkpoint_budget` | `ANAM_CHECKPOINT_BUDGET` | 1 MiB; 0 keeps the fixed width |

Twenty-one more keys, the bounds and waits, are the [config limits](/data-structures/limits.md); `config.example.yaml` lists every one.

The config file is `ANAM_CONFIG` or `<user config dir>/anam/config.yaml`; the example file now lists every key but still carries an old backup method and a stale store list (see [config example is stale](/gaps/config-example-is-stale.md)).
