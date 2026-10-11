---
type: gap
title: config.example.yaml is behind the config keys and names the wrong backup method
description: The example omits checkpoint_budget, backup_dir, backup_keep, max_read_bytes, server, zone and library_dir, repeats the stale operational-store list, and advises sqlite3 .backup while the daemon uses VACUUM INTO.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - config.example.yaml
  - internal/config/config.go
  - internal/backup/backup.go
  - internal/store/ledger/backup.go
---

`LoadOverriding` refuses unknown keys, so the example is safe to copy, only incomplete. The ARCHITECTURE §8 backup advice (`.backup`, or copy with the `-wal` file and no writer) is still a correct manual method; the daemon's `POST /v1/backups` is the built one. See [backup](/components/backup.md) and [config keys](/data-structures/config-keys.md).
