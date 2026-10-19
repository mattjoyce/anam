---
type: term
title: Query
description: The third read after the timeline and fields; a Datalog program evaluated over the fold at a watermark, through a projection, answering with a derived value and a receipt. Decided 2026-10-17 (card 173), not built.
timestamp: 2026-10-19T00:00:00Z
---

A query is a Datalog program run over the [fold](/terms/fold.md) at a [watermark](/terms/watermark.md), through a [projection](/terms/projection.md). The projection decides which whole facets the query may see, as it does for a timeline; the query then relates them: joins across types along [fact-time](/terms/fact-time.md), selection by any payload field the served Definition's schema names, aggregates and windows. Its base relations are the [envelope](/terms/envelope.md) (`entry/7`, carrying `definition_version`, since a payload's shape is the shape of the version it was admitted under) and the payload fields read from the served copy (`attr/3`, for any pointer the schema defines). Bundled rules such as `current/2` and `displaced/2` ship as a named, versioned rule set, and `current/2` is held equal to the [snapshot read](/terms/snapshot-read.md) by a standing test.

The answer is a derived value that hashes, with a [receipt](/terms/receipt.md) naming the watermark and ledger, the projection and its hash, the definition-set version, the rule-set hash and the query text, so `receipt verify` re-runs it. A query reads payload fields from the served copy and never from the store, moves no watermark and writes nothing; the evaluator is read-side and outside the [error kernel](/terms/error-kernel.md). Decided in [a read may select by payload; the store never does](/decisions/a-read-may-select-by-payload-the-store-never-does.md); its surfaces (`anam query`, `/v1/query`, an MCP tool) are [not built](/gaps/the-query-surface-is-decided-and-not-built.md).
