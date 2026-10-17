---
type: purpose
title: Anam, a personal longitudinal record
description: One person's whole-life timeline of facts, assembled verbatim from the systems that already hold their data, owning none of it, with a receipt for everything it shows.
timestamp: 2026-10-17T00:00:00Z
commit: 154f190
primary_workflow: /workflows/admit-a-batch.md
basis: docs
---

## The problem

Data about one person is scattered: a watch measures sleep and heart rate, a scale measures the body, a cuff measures blood pressure, a lab prints a report, and each lives in its own silo. No single place shows the person to themselves over time with the evidence attached, and nothing they would trust to say what it knew, and when.

Anam (Irish for *soul*) is that place for exactly one person. It reads their sources, holds each fragment verbatim as a [Facet](/terms/facet.md) with where it came from and when it was true, and lays them on one timeline. The whole-life read is the default; a span is a lens the reader may add. Every fact accretes and nothing is overwritten, so the past stays as recorded, and every view it serves comes with a [receipt](/terms/receipt.md) proving exactly what was shown at what moment of knowledge.

The repo is the Go service and CLI that own the record. Its sources now arrive through a second repo, `anam-importers`: Python ductile plugins that each know one vendor and submit [envelopes](/terms/envelope.md) over HTTP. This explainer covers both, because the contract between them is where most of the design shows.

## Who it is for

Matt, as the one person recorded, as the operator who runs the daemon on unraid, and as the author of each importer. Agents acting for him are the second consumer: they read projections, with a `reader` token since card 89, and may one day propose findings back through the same gate. There is no second person, so there is no identity graph and no merge.

## What it does not do

- It owns no source fact. Every payload is verbatim from its source and rebuildable; delete Anam and no fact is lost. It does own the continuity, the composed order, which is re-derivable but held nowhere else.
- It never parses a payload into a canonical schema, never joins and never summarises. Weekly averages are projections or derived facts, not admitted facts.
- It never overwrites or deletes. Corrections supersede; retraction is a [tombstone](/terms/tombstone.md) event; the ledger's triggers refuse update and delete for every writer.
- It does not schedule, poll, or know a vendor's tables. Ductile decides when; an [importer](/terms/importer.md) knows the vendor; Anam knows Definitions, the gate and the record.
- It does not retract on its own. An importer reports what its source still holds; the operator retracts, with guards.
- It does not gate on classification or consent yet. The attribute and the pass-through gate exist; the policy is deferred.
- It does not serve multiple people, and it is not a dashboard: it answers "what did I know about this on that date, and from where" rather than "what is the number now".
