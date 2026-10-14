---
type: data-structure
title: Batch event
description: "The ductile event <plugin>.batch an importer emits and an anam_submit instance consumes: envelopes, the manifests as text, a label, and optionally a snapshot part or current_versions."
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/anam_submit/manifest.yaml#commands
anchor: "maxItems: 49"
basis: code
tier: supporting
group: wire
references:
  - to: /data-structures/envelope.md
    how: carries
    card: "1:n"
  - to: /data-structures/definition-manifest.md
    how: carries as text
    card: "1:n"
---

| Field | Type | Set by |
|---|---|---|
| `envelopes` | 1 to 49 envelope dicts | every importer |
| `definitions` | manifest JSON as strings | importers that carry Definitions; `[]` for records |
| `batch` | free label, `<file> i/n` | the importer, echoed into `anam.admitted` |
| `snapshot` | `{id, part, parts, types}` | `snapshot_parts` on resend-everything runs only |
| `current_versions` | bool | the records importer; makes anam_submit stamp versions from GET /v1/definitions. Not declared in anam_submit's manifest (see [anam_submit manifest omits current_versions](/gaps/anam-submit-manifest-omits-current-versions.md)) |
| `documents` | `[{path, file}]` | [anam_documents](/components/importer-documents.md) only; anam_submit posts each file's bytes to `POST /v1/documents` at `path` before the envelopes. On anam-importers origin/master (card 103), not in this checkout |

The ductile protocol is version 2: one JSON request on stdin, one response on stdout, exit. A response is `{status: ok, result, events, logs}` or `{status: error, error, retry, logs}`.
