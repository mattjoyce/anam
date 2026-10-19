---
type: term
title: Projection
description: A versioned template that prunes the canonical by metadata only; a consumer selects one and never defines one.
timestamp: 2026-10-11T00:00:00Z
---

A projection is a versioned template that prunes the [Canonical](/terms/canonical.md) for a purpose and never reshapes or generates. Its knobs are metadata filters: a facet-type allow-list, a datetime window, an endorsement filter, a maximum classification, and a shape, `log` or `snapshot`. It includes or excludes whole facets and never reads a payload to decide. What a reader does with the served facets afterwards, naming fields or running a [query](/terms/query.md), reads the served copy and never widens what the projection admitted.

It is explainable: the verdict that decides whether a facet is served also says which knob excluded it. Served, it is materialised as a content-addressed file with a [receipt](/terms/receipt.md). Templates are bundled as the [projection template](/data-structures/projection-template.md).
