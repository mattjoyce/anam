---
type: data-structure
title: Importers file
description: The YAML the daemon reads once at start naming each importer's source, the SHA-256 of its token, and the types it may submit and register.
timestamp: 2026-10-11T00:00:00Z
resource: internal/config/importers.go#Importer
anchor: "type Importer struct {"
basis: code
tier: supporting
group: contract
references:
  - to: /data-structures/definition-manifest.md
    how: scopes types of
    card: "n:n"
---

```yaml
importers:
  - source: bp-csv
    token_sha256: 9f86d081...      # printf %s "$token" | sha256sum
    types: [health.blood-pressure]
```

| Key | Rule |
|---|---|
| `source` | the source every envelope it submits is admitted as; named once |
| `token_sha256` | 64 lower-case hex; the daemon never holds the token; refused if it equals the operator token's hash |
| `types` | the only types it may submit and register; a type two importers name must be bundled by the binary, or the daemon refuses to start |

Unknown keys are refused. The token itself lives in the gateway's vault under a name only that importer's `anam_submit` instance is granted. The ops `token_scope` table that was meant for this is unused (see [token_scope is a dead table](/gaps/token-scope-table-unused.md)).
