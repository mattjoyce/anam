---
type: term
title: Snapshot (read)
description: "The last-known-value read: the latest facet per series at or before a fact-time, closed series dropped."
timestamp: 2026-10-11T00:00:00Z
---

For a fact-time `D`, the snapshot read keeps the latest facet in each [series](/terms/series.md) at or before `D`, drops a series whose latest is a closing type, and leaves facets without a series alone. It is a pure function of the value it is taken over, so it gives the same answer from the canonical, from an as-of read, or from the composed prefix of hard checkpoints up to `D`'s bucket. A projection with shape `snapshot` serves it.

The word has a second, unrelated meaning on the importer side: see [Snapshot (resend)](/terms/snapshot-resend.md).
