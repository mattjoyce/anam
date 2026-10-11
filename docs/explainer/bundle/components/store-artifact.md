---
type: component
title: Artifact store (internal/store/artifact)
description: "Content-addressed files under artifact_dir: checkpoint values, served values and receipts, written atomically and re-hashed on read."
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/artifact/artifact.go#New
anchor: "func New(dir string) (*Store, error) {"
basis: code
tier: supporting
group: published
depends_on: []
---

A file is `<kind>/sha256-<hex>.json` under an `os.Root`. `Put` writes to a nonce-named temp file with `O_EXCL`, syncs, and renames; existing equal bytes return the digest, existing different bytes are `ErrCorrupt`. `Get` re-hashes on read and refuses a tampered file; `Raw` returns bytes unchecked, so a verify can say what they hash to. Kinds in use: `checkpoint` ([checkpoint value](/data-structures/checkpoint-value.md)), `served`, `receipt` ([receipt](/data-structures/receipt.md)).
