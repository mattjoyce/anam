---
type: term
title: Canonical
description: The live-only value of the fold at a watermark; superseded and tombstoned facets are excluded from it.
timestamp: 2026-10-11T00:00:00Z
---

Canonical names exactly one surface: the live-only [fold](/terms/fold.md) at a [watermark](/terms/watermark.md), every facet whose current status is live. Superseded and tombstoned facets remain in the [ledger](/terms/ledger.md), marked, and are absent from the canonical value by construction.

It is what the normal serve path reads and what [projections](/terms/projection.md) prune. Earlier Anam drafts used the word for the append-everything surface; that is corrected in ARCHITECTURE.md §7.
