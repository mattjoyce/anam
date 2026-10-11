---
type: term
title: Tombstone
description: A tombstoned anam-event marking a facet retracted without deleting it; never a new artifact.
timestamp: 2026-10-11T00:00:00Z
---

A tombstone is a `tombstoned` [anam-event](/terms/anam-event.md) against a facet's current artifact version. No artifact is written, because an Anam artifact has nowhere to carry a status; the status transition is the whole event. The facet stays in the [ledger](/terms/ledger.md) forever, marked, and is absent from the [Canonical](/terms/canonical.md). It advances the [watermark](/terms/watermark.md) like any mutation.

Only `anam retract` writes one; no ingest tombstones, because a row missing from one scan and a source gone look alike from inside an envelope.
