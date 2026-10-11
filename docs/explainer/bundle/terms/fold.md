---
type: term
title: Fold
description: The computation that derives each facet's current status from its anam-events and keeps the live ones.
timestamp: 2026-10-11T00:00:00Z
---

The fold reads the [anam-events](/terms/anam-event.md) up to a [watermark](/terms/watermark.md) and derives each facet's current status (admitted, superseded, tombstoned, endorsed, rejected), keeping the live ones to produce the [Canonical](/terms/canonical.md). Status is never stored; it is always this derivation.

The serve path is nearest [checkpoint](/terms/checkpoint.md) plus replay of the events since, so a read is as fresh as ingest. The whole-person view composes bucket checkpoints; it never re-folds the person. The component is [fold](/components/fold.md).
