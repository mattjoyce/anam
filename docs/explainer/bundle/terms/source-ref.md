---
type: term
title: Source ref
description: The deterministic natural key saying which source row a facet is, composed from identity fields and never from values.
timestamp: 2026-10-11T00:00:00Z
---

A source ref identifies which source row a facet is. The [gate](/components/gate.md) composes it through one function from the [source](/terms/source.md) name and each `identity_key` value, length-prefixed, so two different tuples can never compose one string. `facet_id` is uuidv5 of it, so the same row always maps to the same facet.

Identity decides which fact; the content hash decides whether it changed. Braiding values into the ref would give a corrected reading a new `facet_id`, orphaning the original instead of [superseding](/terms/supersession.md) it.
