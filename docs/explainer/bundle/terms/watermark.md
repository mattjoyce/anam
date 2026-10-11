---
type: term
title: Watermark
description: W, the seq of the newest anam-event; the position every checkpoint and receipt is pinned to.
timestamp: 2026-10-11T00:00:00Z
---

The watermark `W` is the `seq` of the newest [anam-event](/terms/anam-event.md). It advances with every event, a pure status transition included; if it did not, a checkpoint could reuse a value that still lists a retracted facet. The ledger stores no counter: `W` is read as the highest `seq`, and a trigger keeps the sequence gapless.

A watermark means nothing against another ledger, so a [receipt](/terms/receipt.md) carries the `ledger_id` its watermark counts in. `Wᵥ` (`covers_w`) is the watermark a [checkpoint](/terms/checkpoint.md) actually folded.
