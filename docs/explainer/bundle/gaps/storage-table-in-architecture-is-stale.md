---
type: gap
title: The sync_cursor table is never written
description: The operational store creates sync_cursor and the envelope path never writes it, yet ARCHITECTURE §8 lists sync cursors in the derived tier; config.example.yaml still names a stale store list.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - docs/ARCHITECTURE.md
  - internal/store/ops/ops.go
  - internal/store/ops/ops.sql
  - internal/serve/serve.go
  - config.example.yaml
---

The storage table was rewritten on 2026-10-22 (card 215) to name what the code holds, receipts and checkpoint values as rows among them, and card 246 (2026-10-25) replaced the sentence after it that still named bookmarks and logs, and the GLOSSARY entry for the operational store, with the tables `ops.sql` creates. `sync_cursor` exists but the envelope path never writes it. The per-span V and covers_w do match (the `checkpoint` log and `span_head` view). `config.example.yaml` repeats the stale list. See [operational store](/components/store-ops.md).
