---
type: invariant
title: The bucket width is checked once at config load and pinned into the index
description: span.New is the only constructor and refuses an unknown width at startup; the operational store pins the width it was built at and refuses a file built at another, so changing it is a rebuild, not a migration.
timestamp: 2026-10-11T00:00:00Z
resource: internal/config/config.go#Validate
anchor: "if _, err := span.New(c.BucketWidth); err != nil {"
basis: code
tier: detail
enforced_at: internal/store/ops/ops.go#pinBucketWidth
enforced_anchor: "stored, fresh, err := pin(ctx, db, \"bucket_width\", string(w))"
applies_to:
  - /components/config.md
  - /components/span.md
  - /components/store-ops.md
  - /terms/bucket-span.md
tested_by:
  - internal/config/config_test.go#TestLoadRejectsAnUnknownBucketWidth
  - internal/store/ops/ops_test.go#TestOpen_RefusesAnIndexBuiltAtAnotherBucketWidth
---

ARCHITECTURE §7. A width that defaulted quietly would be the worst kind of wrong: the ids it wrote would be well-formed, plausible, and afterwards indistinguishable from correct ones. So a typo dies at config load, not on the ingest path, and `facet_index.bucket_id` is a property of the built index, not of the run. The remedy for a changed width is the one a schema bump gets: delete `tracking.db` and re-run `init`; the derived tier replays, which costs time, never a fact.
