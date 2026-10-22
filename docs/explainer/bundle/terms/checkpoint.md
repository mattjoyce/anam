---
type: term
title: Checkpoint
description: An immutable, content-addressed value of the live-only fold over one bucket at a watermark, there only to bound replay.
timestamp: 2026-10-11T00:00:00Z
---

A checkpoint is the [Canonical](/terms/canonical.md) value of one [bucket span](/terms/bucket-span.md) at a [watermark](/terms/watermark.md), hashed over its bytes and written as a file under that hash. The [mint](/terms/mint.md) produces it solely to bound read-time replay depth; freshness comes from the ledger, not from minting.

A hard checkpoint has a row in the checkpoint log and a row of `checkpoint_value`, and a mint removes it once a later checkpoint of its span subsumes it and no receipt pins it; a soft one is the same value computed on demand and dropped, never kept between requests. Only a hard one is a replay floor. The stored form is the [checkpoint value](/data-structures/checkpoint-value.md).
