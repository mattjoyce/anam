---
type: gap
title: token_scope is a dead table
description: The operational store creates token_scope (source, type) for the gate's authenticated pillar; nothing reads or writes it. Importer scopes come from the importers file.
timestamp: 2026-10-17T00:00:00Z
looked_at:
  - internal/store/ops/ops.sql
  - internal/config/principals.go
  - internal/app/retract.go
  - internal/daemon/envelopes.go
---

`grep token_scope internal/` finds only the DDL. The live scope data is `config.Principal.Types`, built into `gate.StaticScopes` per request; `anam retract` builds its gate with empty scopes, since a tombstone consults none. The Go map that scoped the in-process workers went in card 119. Whether the table is the intended home once token rotation arrives (deferred, §13) or a leftover is not recorded. See [importers file](/data-structures/importers-file.md).
