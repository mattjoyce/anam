---
type: term
title: Role
description: What a principal's token opens, one of reader, importer or operator; each route names the roles it admits and no handler checks who is calling.
timestamp: 2026-10-17T00:00:00Z
---

A `reader` reads the record and publishes reads. An `importer` reads as a reader does and alone submits envelopes, documents and Definitions, as its one source and for its types. The `operator` runs the daemon (minting, backups, projection templates, the inspector) and submits nothing, since it is no source. The three are an invariant of the build, not a setting: see [each route names the roles it admits](/invariants/each-route-names-the-roles-it-admits.md). A reader narrowed to some projections is not built (see [the gap](/gaps/reader-is-not-narrowed-to-projections.md)). Not to be confused with a [source](/terms/source.md), which is where facts come from.
