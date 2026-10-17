---
type: gap
title: config.example.yaml still names the old backup method and a stale store list
description: The example now lists every config key, the limits included, but its ledger comment still advises sqlite3 .backup while the daemon uses VACUUM INTO, and its operational-store list still names token scopes.
timestamp: 2026-10-17T00:00:00Z
looked_at:
  - config.example.yaml
  - internal/config/config.go
  - internal/backup/backup.go
  - internal/store/ledger/backup.go
---

Card 151 added the missing keys: `checkpoint_budget`, `backup_dir`, `backup_keep`, `max_read_bytes`, `server`, `zone`, `library_dir` and the 21 [limits](/data-structures/limits.md) are all in the example, and a script over `Config`'s `mapstructure` tags finds none absent. What remains is two comments. The ledger section advises `sqlite3 ledger.db ".backup ..."`, a correct manual method, while the daemon's `POST /v1/backups` is the built one (see [backup](/components/backup.md)). The operational-store section lists "token scopes" among its tables, but nothing reads or writes that table (see [token_scope is a dead table](/gaps/token-scope-table-unused.md)). `LoadOverriding` refuses unknown keys, so the example is safe to copy. See [config keys](/data-structures/config-keys.md).
