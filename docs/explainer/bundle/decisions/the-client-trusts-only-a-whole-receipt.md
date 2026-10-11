---
type: decision
title: The importer's client stays on its origin and trusts only a whole receipt
description: anam_submit reports a batch admitted only when the 200 body is a complete admission receipt; no redirect is followed, one deadline bounds the whole call, answers are capped at 8 MiB, and a cut answer is retryable.
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/anam_importer/client.py#_call
anchor: "if 300 <= resp.status < 400:"
basis: history
tier: supporting
alternatives:
  - urllib with redirects followed and any 200 taken as success (how it was before cards 112 to 117; a redirect would carry the bearer token to another host, and a truncated 200 looked like success)
affects:
  - /components/importer-base.md
  - /components/anam-submit.md
  - /data-structures/admission-response.md
---

**Context.** Commit 58e692d in anam-importers, cards 112 to 117: "the client stays on its origin and trusts only a whole receipt".

**Choice.** `http.client` instead of `urllib`; a 3xx is a non-retryable error; `admission` checks `w`, `changed` and one result per envelope with a known outcome before `anam.admitted` is emitted; `IncompleteRead` and transport errors are retryable; the timeout is one deadline over the whole call with a sliding socket timeout and a size cap; a request that is JSON but not an object gets a structured error.

**Why.** The gateway retries on the plugin's word; a wrong "admitted" would never be resent, and a token that follows a redirect leaves its origin.

**What it costs.** The client accepts `tombstoned` as an outcome the contract never documents; a 409 is just a non-retryable error with Anam's message.
