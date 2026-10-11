---
type: term
title: Derived span
description: A lens the person names over the timeline, composed on demand from bucket values and never minted.
timestamp: 2026-10-11T00:00:00Z
---

A derived span is an inclusive fact-time interval the person names (`--from`/`--to`), optionally narrowed by type, a saved bookmark, or everything. It is composed on demand by stitching [bucket](/terms/bucket-span.md) values and materialised only when a [receipt](/terms/receipt.md) is served. It is never stored as a unit, never minted, never pulsed.

Conflating it with the bucket span produced the whole-timeline mint that ARCHITECTURE.md §1 retracts: removing the span as a required read filter never required removing it as a mint unit.
