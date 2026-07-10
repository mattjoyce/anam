---
id: 202607102353
type: decision
title: "The ledger's keys are identity-first, fixed-width, and there are no secondary indexes"
created: 2026-07-10
status: evergreen
tags: [anam, decision, dynamodb, keys, indexes, ledger]
project: anam
related: ["[[source-ref-is-identity-not-content]]", "[[fixed-width-timestamps-lexical-order]]", "[[sqlite-is-the-operational-store]]", "[[reference-implementation-mint-cli]]"]
---

# The ledger's keys are identity-first, fixed-width, and there are no secondary indexes

**Decision.** `sk = facet#<facet_id>#v<%06d>` and `rec#<%020d>`. No LSI, no GSIs. The ledger serves
exactly two access paths — the current version of one facet (`begins_with`, descending, limit 1) and
replay-since (`BETWEEN`) — both strongly-consistent base-table scans in the same substrate as the
conditional write. Everything else a reader wants is an index, and indexes are derived, so they live in
the operational store.

## Identity-first — the §6a mistake wearing a sort key

The inherited layout keyed artifacts on `facet#<event_time>#<facet_id>#v<n>`. But `event_time` is a
*value* read out of the payload, and the prior-version lookup would have needed it to locate a facet by
*identity*. A corrected row whose event-time moved lands at a fresh sort key; the lookup misses the
prior version; `commit` writes `v1` again — two live artifacts, one facet, **supersession silently
disabled**. The exact failure [[source-ref-is-identity-not-content]] fixed in the natural key, arriving
through the sort key. Identity-first makes it unrepresentable rather than guarded.

## Fixed-width — verified, not assumed

Sort keys compare lexically: unpadded, `rec#10 < rec#9` and `v10 < v9`. Spiked live: an unpadded
`v1..v12` keyspace queried descending-limit-1 returns **v9**. The prior-version lookup would compare
content hashes against the ninth artifact from the tenth version onward. Same defect family as
[[fixed-width-timestamps-lexical-order]] — and live today in `mint-cli`'s `AllSnapshots`
([[reference-implementation-mint-cli]]).

## No indexes — what "substrate is accidental" has to mean

Dropping the LSI/GSIs dissolved a trade the earlier design was about to take (a 10 GB LSI partition
ceiling on a whole-life store) and left the ledger as: a partition key, a sort key, a conditional
counter. That is *any* ordered key-value store — which is what a swappable substrate has to mean. The
price is honest and named: the fold's indexes are now downstream of catch-up, so catch-up asserts
`last_seq == W` before any fold.

Two more facts pinned by spike: `init` must **seed the counter at 0** (a conditional update against a
missing item fails rather than creating it — otherwise every first commit fails forever), and
`TransactionCanceledException` is **positional**, with `ReturnValuesOnConditionCheckFailure: ALL_OLD`
carrying the existing artifact back for the idempotency hash-compare.
