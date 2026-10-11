---
type: component
title: Receipt (internal/receipt)
description: "The receipt record and its proofs: validate, deterministic bytes, and compare a re-derived value with what the file holds."
timestamp: 2026-10-11T00:00:00Z
resource: internal/receipt/receipt.go#Receipt
anchor: "type Receipt struct {"
basis: code
tier: supporting
group: published
depends_on: []
---

No dependencies: a receipt must be readable from the artifact alone. `Compare` reports two independent proofs, reproduced (the re-derived hash and id set match) and intact (the stored bytes still hash to `served_hash`), with missing and unexpected ids. The record is the [receipt](/data-structures/receipt.md).
