---
type: invariant
title: "W is the anam-event sequence itself: one number, gapless, never stored"
description: The watermark is MAX(seq); a trigger lets an event take only the next seq; every event kind advances it, a pure status transition included.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ledger/ledger.sql#anam_event_seq_follows
anchor: "WHEN NEW.seq IS NOT (SELECT COALESCE(MAX(seq), 0) + 1 FROM anam_event)"
basis: code
tier: supporting
enforced_at: internal/store/ledger/ledger.sql#anam_event_seq_follows
enforced_anchor: "WHEN NEW.seq IS NOT (SELECT COALESCE(MAX(seq), 0) + 1 FROM anam_event)"
applies_to:
  - /components/store-ledger.md
  - /components/gate.md
  - /terms/watermark.md
  - /terms/anam-event.md
tested_by:
  - internal/store/ledger/appendonly_test.go#TestSeq_DatabaseRefusesAGapOrARepeat
  - internal/store/ledger/batch_test.go#TestAdmitBatch_AllocatesAContiguousBlockOfSeqs
---

ARCHITECTURE §7. Because the five kinds are all the anam-events there are and every one can change the live-set, nothing takes a seq without moving the watermark and nothing moves the watermark without taking a seq. The ledger stores no counter. A batch takes a contiguous block, stamped with one anam-time, so anam-time is non-decreasing and ties are broken by seq. A watermark means nothing against another ledger, which is why receipts and the index pin the ledger id.
