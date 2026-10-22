---
type: decision
title: Published reads page by a checked cursor
description: A published read over max_read_bytes is filed once and answered a page at a time, each page cut from the served value; the cursor names its format, the receipt, the offset and the facet there, and is checked against the receipt, not signed.
timestamp: 2026-10-21T00:00:00Z
resource: internal/read/page.go#decodeCursor
anchor: "case ids[c.Offset] != c.FacetID:"
basis: code
tier: detail
tested_by:
  - internal/daemon/page_test.go#TestReceiptPages_WalkTheWholePublishedRead
  - internal/read/page_internal_test.go#TestDecodeCursor
  - internal/app/bound_internal_test.go#TestPublish_OverTheBoundPublishesOnceAndPages
alternatives:
  - Refuse with 413 and ask the reader to narrow (card 88, how it was; an answer that needs every facet had no way to be read over the daemon)
  - Page by re-reading the head at an offset (each page a new read, so pages could disagree with each other and with the receipt)
  - A cursor signed with a daemon key (a key to keep and rotate, for a check the immutable receipt already makes)
  - A /pages sub-route (the first page would be answered in two places)
affects:
  - /components/read.md
  - /components/daemon.md
  - /components/client.md
  - /decisions/reads-over-the-daemon-are-bounded.md
---

**Context.** Card 92, on cards 87 (receipted reads), 88 (the byte bound) and 91 (one facet by its route).

**Choice.** `POST /v1/receipts` files the whole value and its receipt once and answers the first page; `GET /v1/receipts/{hash}?cursor=` reads each later page, and without a cursor the first. A page is the most whole facets from its offset that fit `max_read_bytes` by `Fit`'s measure, cut by `ReceiptPage` from the served value, the first page included. The cursor is base64url of its format, the receipt hash, the offset and the facet id at that offset; a cursor that does not match the receipt it is presented to is 400 `cursor_invalid`. The receipt rides the first page only. 413 is left for one facet over the bound alone, naming it, with the cursor past it on a page.

**Why.** A page is a portion of a receipted value, never a new read of the head, so every page belongs to the same answer whatever Anam learns between pages. The receipt is immutable and content-addressed, so its own `facet_ids` are the cursor's check and no key is needed.

**What it costs.** Each page re-reads and parses the whole served file. The receipt's `facet_ids` sit outside the measure on the first page, as every answer's header does.
