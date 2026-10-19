---
id: 202610190721
type: decision
title: "A read may select by payload; the store never does"
created: 2026-10-19
status: evergreen
tags: [anam, decision, query, payload, projection, datalog, doctrine]
project: anam
related: ["[[verbatim-payload-metadata-separation]]", "[[a-read-names-its-fields-and-gets-back-a-frame]]", "[[a-consumer-selects-a-projection-the-operator-registers-it]]", "[[source-ref-is-identity-not-content]]"]
---

# A read may select by payload; the store never does

Payload opacity is a storage rule, not a ceiling on what a person may ask of their own record.
The ledger keeps every payload as one opaque, content-addressed value, never shredded at ingest,
nothing indexed by field, and the store answers only its fixed forms. A read may still select and
join by any field a served Definition's schema names, because it reads the served copy, after a
projection has decided which whole facets it may see. The query (card 173) is that read: a Datalog
program over the fold at a watermark, through a projection, with a receipt naming the projection,
the rule set and the query text.

Matt decided this on 19 October 2026, amending ARCHITECTURE §8, §9, §12 and §13 together.

## The tension

ARCHITECTURE §8 had said "fields narrow what a projection serves, they do not select by payload",
on the strength of "no read opens a payload". Read against §3, §8 and §9, that sentence had two
rules braided in it: that the store never decomposes a payload, and that a reader may not ask by
one. The first protects the error kernel and the rebuild; the second protected nothing, and on
17 October the supplement questions ("what was I on, and when wasn't I") were joins across types
along fact-time, done in an agent's head because the record could not do them. Matt's question was
why Anam would limit what can be queried. Card 172's bug pulled the same way: two pipeline steps
run in the wrong order, which a rule with the bound as a parameter cannot express.

## Consequences

- Every storage invariant stands: BLOB, verbatim, canonicalised for the hash only, nothing indexed
  by field, the store answering fixed forms.
- A query runs through a projection, never beside one. The projection still only prunes; the
  generating is done over what it served, and the answer is a new value that hashes, not a change
  to any facet. Roles → projections stays the whole of access.
- Two implementations of liveness exist once the evaluator does, `current/2` beside
  `fold.Snapshot`, and a standing test holds them equal. Whether `fold.Fates` later becomes the
  bundled rules is a separate decision, since it would move the evaluator onto the serve path.
- The receipt grows: projection hash, rule-set hash, query text, so `receipt verify` re-runs the
  query over the same value.

## Rejected

- **Limit queries to attributes a Definition declares queryable** (card 173's first draft). A
  per-attribute list would be access below projections, which Anam does not have, and the schema
  already says what every field means.
- **Keep the sentence and answer cross-type questions by hand.** That is the thing a checkable
  query replaces.
