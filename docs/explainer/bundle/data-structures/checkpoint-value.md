---
type: data-structure
title: Checkpoint value file
description: "The live-only value of one bucket at a watermark: a JSON array of fold entries, written under its own sha256 in the artifact directory."
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/artifact/artifact.go#entryName
anchor: 'return path.Join(kind, strings.Replace(digest, "sha256:", "sha256-", 1)+".json"), nil'
basis: code
tier: supporting
group: published
references:
  - to: /data-structures/fold-entry.md
    how: holds
    card: "1:n"
---

| Item | Value |
|---|---|
| Path | `<artifact_dir>/checkpoint/sha256-<64 hex>.json` |
| Bytes | `fold.Value.Bytes()`: `json.Marshal` of the entries sorted by fact-time then facet id |
| Name | `sha256:<hex>` of exactly those bytes, which is also `fold.Value.Hash()` |
| Mode | file 0600, directory 0750 |
| Integrity | re-hashed on `Get`; `mint.Load` also checks the rebuilt value against the row's hash |

The watermark is not in the bytes: a rebuilt ledger reaches the same value at a different W. The docs' `VALUE#<hash>` naming does not appear in code (see [storage table in the architecture doc is stale](/gaps/storage-table-in-architecture-is-stale.md)). Served values and receipts use the same store under `served/` and `receipt/`.
