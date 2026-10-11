---
type: term
title: Receipt
description: Proof of exactly what a projection served at a watermark, verifiable from the artifact alone or against the ledger.
timestamp: 2026-10-11T00:00:00Z
---

A receipt pins the served bytes' hash, the exact facet-id set, the [watermark](/terms/watermark.md) and the `ledger_id` it counts in, the template and definition-set versions, `served_at`, the span, and since card 87 the query a read narrowed by. `receipt verify` re-derives the value from the ledger and re-hashes the file.

Publication is recorded here and nowhere else: a facet's published time is the earliest `served_at` over the receipts naming it. The record is the [receipt](/data-structures/receipt.md).
