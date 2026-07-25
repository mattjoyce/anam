---
id: 202607251300
type: decision
title: "An as-of read fetches by version, and that removes the race rather than handling it"
created: 2026-07-25
status: evergreen
tags: [anam, decision, fold, as-of, ledger, concurrency, receipts]
project: anam
related: ["[[ledger-and-canonical-two-surfaces]]", "[[state-is-checkpoint-plus-replay]]", "[[identity-first-keys-and-no-indexes]]", "[[published-is-a-receipt-not-a-record-event]]", "[[guiding-lens-hickey-armstrong]]"]
---

# An as-of read fetches by version, and that removes the race rather than handling it

**Decision.** A read of the canonical fetches each live facet at **the artifact version the fold
derived** (`ledger.Version(id, n)`), never at whatever version is current
(`ledger.CurrentVersion(id)`). `fold.Read` is `Head()` then `ReadAt(head)`; there is one read path, and
its retry loop is deleted.

## The tension

Two questions were answered by one lookup:

- *Which facets are live?* — folded from record-events up to a watermark.
- *Which version of each?* — asked of the ledger as "the latest".

They agree only while nothing is being written. A supersession landing between the replay and the fetch
returns an artifact the folded events cannot have seen, so the read had to detect the disagreement and
start again: `readAttempts`, an `outpaced` return, `ErrOutpaced`. That retry was the visible cost of the
braid, and it looked like an unavoidable consequence of reading a live system.

## Why fetching by version dissolves it

An artifact version is **immutable once written**. So the answer to "give me v2 of this facet" does not
depend on when it is asked — ingest can land freely during the fetch and change nothing about the
answer. The race is not handled better; it stops existing.

`Replay(0, w)` is already bounded by `w`, and the watermark counter is bumped inside the same
`TransactWriteItems` as the event, so `Head()` returning *n* means events 1..n are all visible. The
whole read is therefore consistent at one watermark with no coordination at all.

## What forced the discovery

R7's receipt. A receipt names a historical watermark, and "reproduces exactly the facet set served at
its watermark" is unprovable if the fetch can only say "the latest" — at that watermark a facet may be
at v2 while current is v3, and the read would spin to `ErrOutpaced`. Building the as-of read *required*
separating the two questions, and the retry fell out as soon as they were apart.

> Governance made the read simpler, not heavier. That is the tell that the two concerns were complected
> rather than genuinely coupled.

## The class, swept

The same braid appeared four times, and one instance was a latent bug rather than only complexity:

| Site | Verdict |
|---|---|
| `fold.fetchLive` | member — retry deleted |
| `catchup.advance` | member — its own `ErrOutpaced` and retry deleted |
| `inspect` (dead facets, ×2) | member, **and wrong**: folded to `st.Version`, fetched current — a facet tombstoned at v2 and later re-captured would render v3's content under v2's status |
| `gate.commit` (×2) | **not** a member — the gate compares content hashes against what is *in force*, so "the latest" is the right question |

`CurrentVersion` is not deprecated. It is the gate's question, and it stays.

## The cost

One more read path on the ledger. Cheap: it is a `GetItem` on the identity-first sort key that already
exists ([[identity-first-keys-and-no-indexes]]) — no index, no scan.
