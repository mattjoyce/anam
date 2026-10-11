---
type: decision
title: Artifact writes are atomic, temp file and rename
description: A checkpoint value, served value or receipt is written to a nonce-named temp file with O_EXCL, synced, and renamed under its hash; equal bytes are idempotent and different bytes at one address are corruption.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/artifact/artifact.go#writeAtomic
anchor: "if err = root.Rename(tmp, name); err != nil {"
basis: code
tier: detail
tested_by:
  - internal/store/artifact/artifact_test.go#TestPut_ConcurrentWritersAndReadersSeeWholeFiles
  - internal/store/artifact/artifact_test.go#TestPut_RefusesDifferentBytesAtSameAddress
alternatives:
  - Write the file in place (replaced in card 95; a reader or a crash could see a partial file under a name that promises a hash)
affects:
  - /components/store-artifact.md
  - /data-structures/checkpoint-value.md
  - /data-structures/receipt.md
---

**Context.** Card 95. The file's name is a promise about its bytes.

**Choice.** All paths resolve under an `os.Root`; a put that finds the name present compares bytes and returns the digest or `ErrCorrupt`; `Get` re-hashes on read. Value before receipt when publishing, so a crash leaves a value no receipt names and never the reverse.

**Why.** Content addressing only means something if the bytes under a name are whole.

**What it costs.** One extra rename per write, and the reason isn't recorded beyond the commit's subject; the tests say what it guarantees.
