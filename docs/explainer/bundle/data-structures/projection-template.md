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
| `types` | yes | allow-list, deny by default; an empty list serves nothing. An entry is a type or a prefix ending in `.` (`activity.exercise-session.`) that admits every type under it at a segment boundary (card 193) |
| `statuses` | yes | endorsement filter, deny by default |
| `max_classification` | yes | Normal 1, Confidential 2, Sensitive 3; unknown ranks above every known |
| `from`, `to` | present, may be empty | `[from, to)` in the clock layout |
| `shape` | no | `log` (default) or `snapshot` |

Today, under `internal/projection/manifests`: `bp` v1 (log, health.blood-pressure), `body` v1 (log, weight, body composition and blood pressure), `activity` v1 (log, the exercise-session leaves, steps and gym visits), `clinical` v1 (log, what a clinician recorded: conditions, encounters, donations, vision prescriptions, lab panels and results), `sleep` v1 (log, health.sleep), `doses` v4 (log, health.medication.dose, dose-missed and compliance), `attention` v1 (log, the three mind.attention types), `intake` v1 (log, health.intake), `meds` v4 (snapshot, health.medication.started, health.medication.dose-changed, health.medication.stopped). All admit admitted and endorsed, over an unbounded window, up to Sensitive except `attention`, which stops at Confidential, the class of its types. The operator registers newer versions of some of these from `projections/` (`body` v2, `activity` v2, `sleep` v5) to add types an importer registers. `GET /v1/coverage` names, for each type, the projections that serve it. The docs' claim that templates live in the operational store does not match (see [storage table in the architecture doc is stale](/gaps/storage-table-in-architecture-is-stale.md)).
