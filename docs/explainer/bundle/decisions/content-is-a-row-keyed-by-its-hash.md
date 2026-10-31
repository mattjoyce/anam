---
type: decision
title: Stored content is a row keyed by the SHA-256 of its bytes
description: A checkpoint value, served value, receipt or saved selection is a row of the operational store keyed by the hash of its own bytes; a put of equal bytes is idempotent, different bytes at one key are corruption, and every read re-hashes.
timestamp: 2026-10-22T00:00:00Z
resource: internal/store/ops/content.go#Put
anchor: "res, err := d.db.ExecContext(ctx, t.put, digest, b)"
basis: code
tier: detail
tested_by:
  - internal/store/ops/content_test.go#TestPut_ConcurrentWritersAndReadersSeeWholeRows
  - internal/store/ops/content_test.go#TestPut_RefusesDifferentBytesAtTheSameKey
  - internal/store/ops/content_test.go#TestTriggers_RefuseWhatImmutableMeans
alternatives:
  - Files under an artifact directory, named by the hash and written by temp file and rename (the design until card 215, when the ledger was in DynamoDB and a content-addressed value fitted neither store; since the ledger moved to SQLite it left a second thing to back up and a folder that was never collected)
affects:
  - /components/store-ops.md
  - /data-structures/checkpoint-value.md
  - /data-structures/receipt.md
---

**Context.** Card 215. The key is a promise about its bytes, as the file name was.

**Choice.** Six tables of one shape (the sixth, `inline_projection`, is card 211's: the bytes a receipt's template hash names, keyed by that hash, first for a projection a query carried and since card 257 for every receipt's template), a hash and the bytes it names (`checkpoint_value` derived; `receipt`, `served_value`, `saved_selection`, `saved_selection_code`, `inline_projection` authored). `Put` inserts and, if the key is there, compares the bytes and returns the digest or `ErrCorrupt`; `Get` re-hashes on read. The authored five refuse an update and a delete by trigger; `checkpoint_value`, a cache, refuses an update and lets a mint delete a value it no longer reads. Value before receipt when publishing, so a crash leaves a value no receipt names and never the reverse.

**Why.** Content addressing only means something if the bytes under a key are whole, and a backup is only one consistent thing if the receipts are in the same file as everything else Anam authored.

**What it costs.** A schema bump, so a store from before is rebuilt and the folder moved in (`anam move-artifacts`). The triggers refuse an update and a delete but not an insert that replaces, as in the registered tables; the store's own writes never replace.
