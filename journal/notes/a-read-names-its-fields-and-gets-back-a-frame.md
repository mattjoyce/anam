---
id: 202610122300
type: decision
title: "A read names the fields it wants and gets back a frame"
created: 2026-10-12
status: evergreen
tags: [anam, decision, read, api, projection, receipts, mcp]
project: anam
related: ["[[published-is-a-receipt-not-a-record-event]]", "[[ledger-and-canonical-two-surfaces]]", "[[verbatim-payload-metadata-separation]]"]
---

# A read names the fields it wants and gets back a frame

**Decision (Matt, 2026-10-12, card 138).** A read may name its **fields**, a temporary subset of the
type's schema given as JSON pointers into the payload, and nothing else of the payload comes back.
With fields, the answer is a **frame** (`columns` and `rows`) in place of a list of facets. Matt's
picture: only what we want shines through the projection.

## The tension

Reads were whole facets only. That was simple and honest, but a model asking "what was my best rowing
week" got every stroke of every session. A month of `exercise.session` was 184 KB whole and 3.7 KB
cut to distance, duration and erg type, 2.7 KB as a frame. The largest single row, 752 strokes, was
72 KB. Under card 88's 512 KiB read bound, three months of Concept2 sessions was the most a whole read
could answer, so the question could not be asked at all.

The pull the other way was doctrine. "A projection prunes and a consumer selects" had kept consumers
from shaping what Anam serves. Fields keep that intact because they only take away. They run after the
projection and the narrowing have admitted whole facets, so they can neither resurrect a facet nor
exclude one, and the envelope is never cut. `content_hash` stays the whole payload's, so each row
still names its fact.

The noun was argued too. For a few hours it was "mask". Matt rejected it, because a mask names what
should not show, and `docs/API.md` already used "element masking" for hiding. "Fields" names what
should show.

## The rule

- `fields` is a list of JSON pointers, the vocabulary Definitions already use for `fact_time.pointer`
  and `identity_key`. Each must resolve in the Definition schema of every type the read can serve, or
  the read fails, as an unknown filter does. Validation is against the schema, not the data.
- The frame's columns are the envelope's attributes, then the pointers as given. A row lacking a
  pointer has null there. Columns are flat because a frame is a relation, which reads straight into a
  DataFrame.
- A receipt carries the fields, its `served_hash` is over the frame, and verify replays them. A receipt
  without fields verifies as before.
- `max_read_bytes` is measured over the frame, and a 413's `narrow` lists `fields` when the read named
  none.

Datomic's pull is the model: a pattern naming the shape wanted, over an immutable database at a point.
Query over fields (aggregates, windows) is a later step and was kept out of this decision.
