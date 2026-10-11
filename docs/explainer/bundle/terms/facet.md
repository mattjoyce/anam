---
type: term
title: Facet
description: The atomic unit Anam keeps, a source fragment held verbatim plus the metadata that governs it.
timestamp: 2026-10-11T00:00:00Z
---

A Facet is a captured source fragment plus its governing metadata, and deliberately not a normalised domain entity. It has three parts: attributes about itself (id, type, classification, fact-time, series stamp, binding, provenance), a payload descriptor (source schema and version, content hash, payload kind, media type), and the payload, which is either the verbatim structured body or a native reference to a binary in its source.

The payload is opaque to Anam: stored as bytes, never parsed into a canonical schema. Confidence, type and provenance are attributes, never payload fields. A Facet is a value, so its current status is never stored on it; the [Fold](/terms/fold.md) derives status from [anam-events](/terms/anam-event.md).

A Facet's shape in the ledger is the [facet artifact row](/data-structures/facet-artifact.md). Its contract per type is the [Facet Definition](/terms/facet-definition.md).
