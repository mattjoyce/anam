---
type: term
title: Fact-time
description: When a fact was true in the world; the axis the timeline is ordered by, promoted out of the payload at the gate.
timestamp: 2026-10-11T00:00:00Z
---

Fact-time is when a fact happened in the world: the reading, the night's sleep. It determines where a [facet](/terms/facet.md) sits on the timeline and which [bucket span](/terms/bucket-span.md) it belongs to.

It is extracted from the payload at the [gate](/components/gate.md) by the Definition's `time_semantics.fact_time` JSON Pointer, read in the Definition's declared zone, and promoted to a first-class governed field. That promotion is what lets the timeline be ordered and a window filtered without anything opening a payload. Narrowing by fact-time (`--from`/`--to`) is a [derived span](/terms/derived-span.md), not an as-of.
