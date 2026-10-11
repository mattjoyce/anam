---
type: component
title: Backup (internal/backup)
description: POST /v1/backups copies both SQLite files with VACUUM INTO and the artifact and library directories into a dated folder, keeping backup_keep of them.
timestamp: 2026-10-11T00:00:00Z
resource: internal/backup/backup.go#Take
anchor: "func Take(ctx context.Context, d Deps) (api.Backup, error) {"
basis: code
tier: detail
group: tools
depends_on:
  - /components/store-ledger.md
---

Runs as a write on the daemon's queue, so no admission is mid-flight. Each store copies itself in one read transaction; a `.partial` directory is renamed on completion; older sets are pruned to `backup_keep`. There is no `anam backup` CLI command, only the route; and the ledger's `.backup` advice in `config.example.yaml` names a different method (see [config example is stale](/gaps/config-example-is-stale.md)).
