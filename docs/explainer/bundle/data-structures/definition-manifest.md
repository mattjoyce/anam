---
type: data-structure
title: Definition manifest
description: "The JSON contract for one facet type: meta with identity, time, classification, validation mode and series, and a closed JSON Schema 2020-12."
timestamp: 2026-10-17T00:00:00Z
resource: internal/def/def.go#manifest
anchor: "type manifest struct {"
basis: code
tier: supporting
group: contract
references: []
---

| `meta` key | Meaning | Default |
|---|---|---|
| `facet_type` | the type; a bundled file must be named `<type>.json` | required |
| `definition_version` | at least 1; envelopes must name the current one | required |
| `classification` | Normal, Confidential, Sensitive | required |
| `validation_mode` | `strict` (schema as written) or `verbatim` (no schema) | required |
| `source_schema`, `schema_version`, `media_type`, `payload_kind` | the descriptor copied onto each facet | 1, `application/json`, `verbatim` |
| `time_semantics.fact_time` | `{pointer, layout, zone}`: a JSON Pointer, a Go time layout, an IANA zone | required |
| `identity_key` | JSON Pointers, in order, composed into the source ref | required, at least one |
| `min_binding_assurance` | the floor a binding must meet | `medium` |
| `series` | `{name, key[], closes}` | none |
| `kind` | what sort of fact the type records, for readers: `measure`, `reading` or `event` (card 176); the gate ignores it | none |
| `family` | the lower-case hyphenated word a reader groups the type under (`meds`, `sleep`); the inspector draws the type on that row (card 176); the gate ignores it | none |

Schema rules at load: top-level object with properties; every required property refuses null and the empty string; the top-level field behind every keying pointer is required; every object sets `additionalProperties`; no `$ref` leaves the manifest; nothing is fetched.

A version that only loosens a rule leaves older values valid: `note.annotation` v3 (card 157) made `about` optional, so v1 and v2 notes stay valid and a note may name no facet yet, because an account can come before the observation it describes. Identity, mechanism and fact time did not change (v2 timed a note by when it was written, not by the moment it is about). Since the rename of card 147 that body is `record.note` v1, because a renamed type starts again at v1 under its new name.

Type names follow one rule (card 147): `domain.concept`, with the head one of a closed set of eight, `health`, `activity`, `place` and `record` in use and `social`, `money`, `media` and `work` reserved. A new fact joins one of the eight and never earns a new head; growth is in the second segment and in `family`. A Definition declares `kind` and `family` for readers (card 176).

Where they live: 32 bundled under `internal/def/manifests`, embedded (three of them the retired health-db types, kept so their facets still read), with the 45 earlier versions a facet may have been admitted under in `manifests/history`. A bundled type may also be carried by an importer, which re-sends the same body at the same version and is answered 200 unchanged (card 139 piece 1). Importers carry the rest under `<importer>/definitions/` in the same format and register them: `place.stay`, `place.track`, `activity.day`, `health.cpap-session`, `health.measurement` and `record.link`, a file Anam remembers but does not hold (card 165). Stored in the operational store's `definition` (bundled, every version) and `registered_definition` (immutable) tables.
