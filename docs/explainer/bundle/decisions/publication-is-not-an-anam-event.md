---
type: decision
title: Publication is not an anam-event
description: Serving changes nothing Anam knows, so there is no published event; a receipt records what was shown, and a facet's published time is the earliest served_at over the receipts naming it.
timestamp: 2026-10-11T00:00:00Z
resource: internal/serve/serve.go#Publish
anchor: "if _, err := d.Published.Put(ctx, receipt.ServedKind, servedBytes); err != nil {"
basis: docs
tier: supporting
tested_by:
  - internal/serve/serve_test.go#TestRun_LeavesTheLedgerUntouched
alternatives:
  - A sixth anam-event kind, published (refused; it would braid what Anam knows with what Anam has shown, the same complecting the ledger-versus-canonical split untangles)
affects:
  - /components/serve.md
  - /components/receipt.md
  - /terms/receipt.md
  - /terms/anam-event.md
---

**Context.** Every remaining event kind maps one-to-one onto a derivable status; publication would not.

**Choice.** Serving writes two content-addressed rows of the operational store, a served value and a receipt, and nothing in the ledger. The receipt pins the served hash, the facet-id set, the watermark and the ledger it counts in, the template and definition-set versions, served_at, the span and the narrowing query.

**Why.** One word, one surface: the ledger is what Anam knew, receipts are what it showed.

**What it costs.** Published time moves out of the ledger into the operational store's authored tier, which is backed up, not rebuilt: lose the receipts and published time is gone. There is no separate receipt index; listing reads the `receipt` table and sorts by served_at.
