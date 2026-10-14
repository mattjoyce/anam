---
type: component
title: anam_submit
description: "The one generic plugin that posts batches: makes sure Anam publishes the batch's Definitions, submits under the importer's token, and raises the absent alert on a completed snapshot."
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/anam_submit/run.py#handle
anchor: "def handle(request: Dict[str, Any]) -> Dict[str, Any]:"
basis: code
tier: supporting
group: importers
depends_on:
  - /components/importer-base.md
  - /components/daemon.md
---

One instance per importer in the gateway, each with its own `token_secret` from the vault, so the token names the source and no importer holds it. `handle` reads a [batch event](/data-structures/batch-event.md), parses the manifests it carries, calls `ensure_published`, stamps current versions when the batch says `current_versions` (the records importer), submits, and emits `anam.admitted` with the outcome counts. A 422 is a failed run, not retried. When the part completes a snapshot and the report says facets are absent, it emits `anam.snapshot_absent` keyed on the snapshot id, and retracts nothing. `health` lists Anam's Definitions. On origin/master (anam card 103) it also posts the bytes of each file a batch's `documents` names to `POST /v1/documents` before the envelopes; see [hand Anam a document](/workflows/hand-anam-a-document.md).
