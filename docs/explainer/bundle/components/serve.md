---
type: component
title: Serve (internal/serve)
description: "Publishes a projection: canonical at head, prune, write the served value then its receipt, and verifies a receipt by re-deriving."
timestamp: 2026-10-11T00:00:00Z
resource: internal/serve/serve.go#Publish
anchor: "func Publish(ctx context.Context, d Deps, t projection.Template, w ledger.Watermark, q *receipt.Query,"
basis: code
tier: supporting
group: published
depends_on:
  - /components/mint.md
  - /components/fold.md
  - /components/projection.md
  - /components/receipt.md
  - /components/store-ops.md
  - /components/store-ledger.md
---

`Run` serves a whole template; `Publish` files value then receipt, so a crash between them leaves a value no receipt names and never a receipt without its value; `Narrow` applies a read's query after the template; `Verify` re-derives at the receipt's watermark and refuses a moved template version or another ledger; `List` and `Show` read the `receipt` and `served_value` tables, which are the only receipt index; `List` takes a `ListFilter` of principal and served-time window, applied to each parsed receipt. `Deps.Reader` is the principal a receipt filed through the Deps names, empty for every ordinary publish. Traced in [publish a projection](/workflows/publish-a-projection.md).
