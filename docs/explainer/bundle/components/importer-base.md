---
type: component
title: Importer base (anam_importer)
description: "The shared Python package every importer starts from: the ductile protocol, Definitions and local validation, envelopes and batches, and the HTTP client."
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/anam_importer/envelopes.py#envelope
anchor: "def envelope("
basis: code
tier: supporting
group: importers
depends_on: []
---

| Module | Gives |
|---|---|
| `ductile.py` | protocol 2: one JSON request on stdin, one JSON response on stdout; `Failure(message, retry)` for a source that failed; every exception becomes a structured error |
| `definitions.py` | `load` the folder's `definitions/*.json`, `Definition.errors` with jsonschema, `ensure_published` (GET the version, POST only on 404, stop on a different body) |
| `envelopes.py` | `envelope` (type, version, payload, asserted_person, and nothing else), `check` before anything leaves, `batches` at `MAX_BATCH = 49`, `snapshot_parts` |
| `client.py` | `Client` over `http.client`: bearer token, one deadline over the whole call, 8 MiB answer cap, no redirect followed, a 200 accepted only as a whole [admission receipt](/data-structures/admission-response.md) |

An envelope names no source and no `source_ref`: the token names the source and the gate composes identity. The in-tree plugins built on it: [anam_bp_csv](/components/importer-bp-csv.md), [anam_dreamstation](/components/importer-dreamstation.md), [anam_records](/components/importer-records.md), and `anam_withings`, which reads the withings plugin's SQLite database read-only, shapes each measurement to `health.measurement` v4 with the value written as the shortest decimal that round-trips (`29.371`, not `29.371000000000002`), resends everything on a reconcile, and treats a locked database as retryable. [anam_documents](/components/importer-documents.md) is built on it too, on origin/master. Retry is decided by status: 5xx and 429 and transport errors retry; 422, 409 and other 4xx do not.
