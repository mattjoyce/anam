---
id: 202607100003
type: decision
title: "Person-substrate grain — the person is the storage substrate"
created: 2026-07-10
status: evergreen
tags: [anam, decision, storage, grain]
project: anam
related: ["[[the-inversion]]", "[[mint-unit-whole-timeline]]"]
---

# Person-substrate grain — the person is the storage substrate

**Decision.** Storage is partitioned by the person (`person#<id>`); all their facets co-locate on one
event-time-ordered **timeline**. The person is the *substrate*, and — following [[the-inversion]] — the
**default read**.

## Note on lineage

Arch already person-partitions (its §5 calls the Person the substrate). So the grain itself isn't the
novelty; the novelty is *access*. Arch reads the substrate only through a required episode span; Anam
reads it whole. Same shelf, different default reach.

An earlier REC framing (pre-Arch) had swung to *Episode-as-unit, Person-as-index* to escape longitudinal
cost. Anam consciously walks back from that to person-substrate, because for one person the longitudinal
cost is the point, not a tax.

## Consequences

- Single partition key means no cross-person query, no identity graph, no merge — all dropped.
- The `person#<id>` constant keeps a multi-person seam open in the keys, but nothing exercises it.
- Timeline reads are a `Query` with `SK BETWEEN` over the person partition — cheap and range-ordered
  because keys embed a [[fixed-width-timestamps-lexical-order|fixed-width UTC timestamp]]
  (lexical == chronological).
- The person partition is what Arch's keystone ADR 0035 removed. Anam re-adopts it deliberately; see
  [[the-inversion]] and [[anam-owns-the-continuity]].
