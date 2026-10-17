---
type: data-structure
title: Batch submission
description: "The JSON body of POST /v1/envelopes: 1 to 49 envelopes and an optional snapshot label; any unknown field is a 400."
timestamp: 2026-10-11T00:00:00Z
resource: internal/api/importers.go#Submission
anchor: "type Submission struct {"
basis: code
tier: supporting
group: wire
references:
  - to: /data-structures/envelope.md
    how: carries
    card: "1:n"
---

| Field | Type | Required |
|---|---|---|
| `envelopes` | list of submitted envelopes | yes, 1 to 49 |
| `snapshot` | `{id, part, parts, types[]}` | no; only on a resend-everything run |

A submitted envelope:

| Field | Required |
|---|---|
| `type` | yes |
| `definition_version` | yes, at least 1 |
| `payload` | yes, raw JSON |
| `source_ref` | no |
| `asserted_person` | no |
| `span_hint` | no |

There is no `source` field. The body is bounded at 49 × `max_payload_bytes` plus 1 MiB (about 16 MB by default) and must be exactly one JSON value. The snapshot id is 1 to 128 of letters, digits and `. _ : -`; every type in it must be the importer's and every envelope's type one of them. Answered by the [admission response](/data-structures/admission-response.md).
