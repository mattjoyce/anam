---
type: gap
title: The storage table in ARCHITECTURE §8 lists things the code does not have
description: ARCHITECTURE §8's prose still names a span registry, saved bookmarks, GC state and an operational log, and a receipt index; the code has none of them, and the sync cursor table is never written.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - docs/ARCHITECTURE.md
  - internal/store/ops/ops.go
  - internal/serve/serve.go
  - config.example.yaml
---

The storage table was rewritten on 2026-10-22 (card 215) to name what the code holds, receipts and checkpoint values as rows among them; the sentence after it still says Anam has bookmarks and logs, and the ops schema has no bookmark, GC or log table; `sync_cursor` exists but the envelope path never writes it. The per-span V and covers_w do match (the `checkpoint` log and `span_head` view). `config.example.yaml` repeats the stale list. See [operational store](/components/store-ops.md).
