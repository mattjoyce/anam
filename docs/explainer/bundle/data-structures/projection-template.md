---
type: data-structure
title: Projection template
description: "An embedded JSON manifest naming a versioned prune: types, statuses, a classification ceiling, a fact-time window and a shape."
timestamp: 2026-10-11T00:00:00Z
resource: internal/projection/projection.go#Template
anchor: "type Template struct {"
basis: code
tier: supporting
group: contract
references: []
---

| Key | Required | Semantics |
|---|---|---|
| `name` | yes | the file must be `<name>.json` |
| `version` | yes, at least 1 | pinned into receipts |
| `description` | no | |
| `types` | yes | allow-list, deny by default; an empty list serves nothing |
| `statuses` | yes | endorsement filter, deny by default |
| `max_classification` | yes | Normal 1, Confidential 2, Sensitive 3; unknown ranks above every known |
| `from`, `to` | present, may be empty | `[from, to)` in the clock layout |
| `shape` | no | `log` (default) or `snapshot` |

Three today, under `internal/projection/manifests`: `bp` v1 (log, health.blood-pressure), `sleep` v1 (log, health.sleep), `meds` v1 (snapshot, med.started, med.dose-changed, med.stopped). All admit admitted and endorsed, up to Sensitive, over an unbounded window. The docs' claim that templates live in the operational store does not match (see [storage table in the architecture doc is stale](/gaps/storage-table-in-architecture-is-stale.md)).
