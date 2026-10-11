---
type: term
title: Facet Definition
description: The versioned contract for one facet type, from its closed JSON Schema to how identity and fact-time are read from a payload.
timestamp: 2026-10-11T00:00:00Z
---

A Facet Definition is the per-type contract, versioned independently of the facets admitted under it. Its `meta` names the type, version, `identity_key`, `time_semantics`, classification, validation mode and series; its `schema` is closed JSON Schema draft 2020-12 that declares every field a payload may hold and refuses the rest.

Definitions are governance, not record: they live in the [operational store](/terms/operational-store.md), every version, and never in the ledger. A type is provisioned from one of two places, the JSON bundled in the binary or an [importer](/terms/importer.md) that registers it, never both. The on-disk format is the [Definition manifest](/data-structures/definition-manifest.md).
