---
type: gap
title: token_scope is a dead table
description: The operational store creates token_scope (source, type) for the gate's authenticated pillar; nothing reads or writes it. Importer scopes come from the importers file and the built-in workers' scopes from a Go map.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - internal/store/ops/ops.go
  - internal/config/importers.go
  - internal/app/ingest.go
  - internal/daemon/envelopes.go
---

`grep token_scope internal/` finds only the DDL. The live scope data is `config.Importer.Types`, built into `gate.StaticScopes` per request, and `mvpScopes` in `internal/app`. Whether the table is the intended home once token rotation arrives (deferred, §13) or a leftover is not recorded. See [importers file](/data-structures/importers-file.md).
