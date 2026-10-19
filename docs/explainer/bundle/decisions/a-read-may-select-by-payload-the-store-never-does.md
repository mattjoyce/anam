---
type: decision
title: A read may select by payload; the store never does
description: Payload opacity is a storage rule, not a ceiling on what a person may ask of their own record; a query selects and joins by any schema field, read from the served copy through a projection, and the store still never decomposes or indexes a payload.
timestamp: 2026-10-17T00:00:00Z
resource: docs/ARCHITECTURE.md
anchor: "The **store** never selects by payload; a **read**"
basis: docs
tier: supporting
alternatives:
  - Limit queries to attributes a Definition declares queryable (the first draft of card 173; refused, since a per-attribute list would be access below projections, which Anam does not have, and the schema already says what every field means)
  - Keep the §8 sentence and answer cross-type questions by hand, as the 2026-10-17 supplement questions were (refused; the joins were being done in an agent's head, which is the thing a checkable query replaces)
affects:
  - /terms/query.md
  - /terms/projection.md
  - /invariants/projections-prune-never-reshape.md
  - /invariants/metadata-payload-separation.md
  - /components/serve.md
---

**Context.** ARCHITECTURE §8 said "fields narrow what a projection serves, they do not select by payload", on the strength of "no read opens a payload". Read against §3, §8 and §9, that sentence governed storage: the payload is a BLOB, never shredded at ingest, nothing indexed by field. Card 138's fields already read the served copy. On 2026-10-17 the supplement questions ("what was I on, and when wasn't I") were joins across types along fact-time, answered by hand, and card 172's bug was two pipeline steps in the wrong order, which a rule with the bound as a parameter cannot express. Matt asked why Anam would limit what can be queried.

**Choice.** The store never selects by payload; a read may, over the served copy, through a projection. The [query](/terms/query.md) is the third read: a Datalog program over the fold at a watermark, with the envelope and every schema field of the served Definition as relations, bundled rules for Anam's own time semantics, and a receipt that names the projection, rule set and query. §8, §9, §12 and §13 of ARCHITECTURE were amended together on 2026-10-19.

**Why.** Opacity and queryability were two rules in one sentence. Separating them keeps every storage invariant and removes a ceiling that protected nothing: a projection is the access boundary, and a query that runs through one sees exactly what a timeline would.

**What it costs.** A second implementation of liveness (`current/2` beside `fold.Snapshot`) that a standing test must hold equal; a query engine that reads whole served payloads, so the read bound and the deadline are the only thing between an agent and a slow answer; and a receipt that grows by the projection hash, the rule-set hash and the query text. Whether `fold.Fates` later becomes the bundled rules is a separate decision, because it would move the evaluator onto the serve path.
