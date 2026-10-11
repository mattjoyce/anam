---
type: term
title: Ledger
description: Anam's append-only record of everything it admitted, every facet kept and marked at its current status.
timestamp: 2026-10-11T00:00:00Z
---

The ledger is Anam's system of record: an append-only log of every [facet](/terms/facet.md) ever admitted, with superseded and tombstoned ones retained and marked, plus every [anam-event](/terms/anam-event.md). Facts are added and re-statused, never modified in place, never deleted. It is authoritative about what Anam knew, never about the world; the sources are.

It is one SQLite file of its own, apart from the [operational store](/terms/operational-store.md), with triggers that refuse update and delete for every writer. Forensic reads, as-of reads and rebuild go to it; the [Canonical](/terms/canonical.md) is derived from it. The component is [ledger store](/components/store-ledger.md).
