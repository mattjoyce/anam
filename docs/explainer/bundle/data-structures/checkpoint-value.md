---
type: data-structure
title: Checkpoint value row
description: "The live-only value of one bucket at a watermark: a JSON array of fold entries, a row of checkpoint_value keyed by its own sha256."
timestamp: 2026-10-22T00:00:00Z
resource: internal/store/ops/content.go#newContentTable
anchor: 'put:  q(`INSERT INTO {table} (hash, body) VALUES (?, ?) ON CONFLICT (hash) DO NOTHING`),'
basis: code
tier: supporting
group: derived
references:
  - to: /data-structures/fold-entry.md
    how: holds
    card: "1:n"
---

| Item | Value |
|---|---|
| Table | `checkpoint_value (hash, body)`, derived tier: a rebuild may drop it and a mint writes it again |
| Bytes | `fold.Value.Bytes()`: `json.Marshal` of the entries sorted by fact-time then facet id |
| Key | `sha256:<hex>` of exactly those bytes, which is also `fold.Value.Hash()` |
| Integrity | re-hashed on `Get`; `mint.Load` also checks the rebuilt value against the row's hash |
| Removal | refused by trigger while a `checkpoint` row names the value; a mint removes the values nothing names (`CollectCheckpoints`) |

The watermark is not in the bytes: a rebuilt ledger reaches the same value at a different W. Until card 215 the value was a file, `checkpoint/sha256-<hex>.json` under `artifact_dir`. Receipts, served values and saved selections use the same shape in authored tables.
