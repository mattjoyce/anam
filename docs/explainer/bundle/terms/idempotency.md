---
type: term
title: Idempotency
description: Re-ingesting the same source row is a no-op; what makes ingest crash-clean and a resend always safe.
timestamp: 2026-10-11T00:00:00Z
---

Same [source ref](/terms/source-ref.md) and same content hash is a no-op; the same `(facet_id, version)` with the same bytes is our own earlier write. Guaranteed by the deterministic facet id and an atomic conditional write, so a worker may crash mid-batch and be re-run to known-good, and an importer unsure whether a batch landed sends it again. See [commit is idempotent and atomic](/invariants/commit-is-idempotent-and-atomic.md).
