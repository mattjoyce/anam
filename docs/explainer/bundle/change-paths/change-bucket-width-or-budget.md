---
type: change-path
title: Change the bucket width or the checkpoint budget
description: The budget can change at any time and the next mint settles it; the width is pinned into the index, so changing it is a rebuild of tracking.db and a full mint.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ops/ops.go#pinBucketWidth
anchor: 'stored, fresh, err := pin(ctx, db, "bucket_width", string(w))'
basis: code
tier: detail
touches:
  - /data-structures/config-keys.md
  - /components/span.md
  - /components/mint.md
  - /components/store-ops.md
  - /invariants/bucket-width-is-checked-once-and-pinned.md
  - /decisions/bucket-on-fact-time-subdivide-by-budget.md
  - /gaps/weekly-width-disables-composed-reads.md
---

**Budget** (`checkpoint_budget`, bytes). Lower it and the next mint finds every checkpoint file over the new budget by its size and splits those buckets along the calendar; raise it and nothing merges, because splits only add. Set it to 0 to keep the fixed `bucket_width` with no subdivision, which also makes every mint a full one.

**Width** (`bucket_width`, Monthly or Weekly). The operational store refuses a file built at another width. The order: stop the daemon, change the config, delete `tracking.db`, run `anam init` to rebuild it, start the daemon (its first write replays the ledger into the index), then `anam checkpoint --full`. Facets and events are untouched; the checkpoint values under the old ids go with the deleted `tracking.db`, and the rebuilt one holds only the values the new mint writes.

Weekly is accepted but does not subdivide a year, so the composed read and the incremental mint refuse it and fold in full; see the gap before choosing it.
