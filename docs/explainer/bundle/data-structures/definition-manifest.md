---
type: data-structure
title: Definition manifest
description: "The JSON contract for one facet type: meta with identity, time, classification, validation mode and series, and a closed JSON Schema 2020-12."
timestamp: 2026-10-11T00:00:00Z
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

Schema rules at load: top-level object with properties; every required property refuses null and the empty string; the top-level field behind every keying pointer is required; every object sets `additionalProperties`; no `$ref` leaves the manifest; nothing is fetched.

Where they live: 28 bundled under `internal/def/manifests`, embedded; importers carry theirs under `<importer>/definitions/` in the same format and register them. Stored in the operational store's `definition` (bundled, every version) and `registered_definition` (immutable) tables.
