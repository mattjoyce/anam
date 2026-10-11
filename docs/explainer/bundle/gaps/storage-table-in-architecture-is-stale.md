---
type: gap
title: The storage table in ARCHITECTURE §8 lists things the code does not have
description: VALUE#<hash> files, projection templates in SQLite, a receipt index, a span registry, saved bookmarks, sync cursors in use, GC state and an operational log are named; the code has checkpoint/sha256-<hex>.json files, embedded templates, receipts listed from the directory, and none of the rest.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - docs/ARCHITECTURE.md
  - internal/store/ops/ops.go
  - internal/store/artifact/artifact.go
  - internal/serve/serve.go
  - config.example.yaml
---

`grep VALUE# internal/` is empty; `grep receipt internal/store/ops/` is empty; the ops schema has no bookmark, GC or log table; `sync_cursor` exists but the envelope path never writes it. The per-span V and covers_w do match (the `checkpoint` log and `span_head` view). `config.example.yaml` repeats the stale list. See [operational store](/components/store-ops.md).
