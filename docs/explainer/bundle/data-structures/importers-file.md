---
type: data-structure
title: Importers file
description: The YAML the daemon reads once at start naming each principal, its role, the SHA-256 of its token and, for an importer, the source it submits as and the types it may submit and register, or any_type.
timestamp: 2026-10-19T00:00:00Z
resource: internal/config/principals.go#Principal
anchor: "type Principal struct {"
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
principals:
  - name: assistant
    role: reader
    token_sha256: 2c26b46b...
```

| Key | Rule |
|---|---|
| `importers[].source` | the source every envelope it submits is admitted as; named once. Its name defaults to the source and its role is `importer` |
| `principals[].name` | who the principal is in a log or a refusal; used once; `operator` is reserved for the `token_file` token |
| `principals[].role` | `reader`, `importer` or `operator`; any other value is refused |
| `token_sha256` | 64 lower-case hex; the daemon never holds the token; shared by no two principals and refused if it equals the `token_file` token's hash |
| `types` | an importer's only: the types it may submit and register; a type two importers name must be bundled by the binary, or the daemon refuses to start |
| `any_type` | an importer's only, in place of `types`: it submits every type the daemon knows, registers none and stores no document (card 177, the person speaking as `conversation`); with `types` as well, refused |

An importer in `principals` states `role: importer`, a source and types or `any_type`; any other role has no source and no types, and an `importers` entry that states a role other than importer is refused. Unknown keys are refused. The token itself lives in the gateway's vault under a name only that importer's `anam_submit` instance is granted. The ops `token_scope` table that was meant for this is unused (see [token_scope is a dead table](/gaps/token-scope-table-unused.md)). What each role opens is on [principals and roles](/components/principals.md).
