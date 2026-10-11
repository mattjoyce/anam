---
type: term
title: Snapshot (resend)
description: An importer's labelled resend-everything run, from which Anam reports what the source no longer holds.
timestamp: 2026-10-11T00:00:00Z
---

An [importer](/terms/importer.md) never tombstones. A resend-everything run labels each batch as one part of a snapshot (`id`, `part`, `parts`, `types`); Anam keeps the parts and the facet ids they carried, and once every part has arrived reports what is live from that source, untouched since the run began, and not carried. The operator then retracts with `anam retract <source>`. A part is bound to its envelopes: resent with others it is a 409.

Unrelated to the [snapshot read](/terms/snapshot-read.md), which shares the word. The record is the [snapshot part](/data-structures/snapshot-part.md).
