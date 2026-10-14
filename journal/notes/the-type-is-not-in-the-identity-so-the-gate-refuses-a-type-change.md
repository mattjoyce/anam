---
id: 202610102100
type: decision
title: "The type is not in the identity, so the gate refuses a facet changing type"
created: 2026-10-10
status: evergreen
tags: [anam, decision, gate, identity, source-ref, error-kernel]
project: anam
related: ["[[identity-is-definition-owned]]", "[[source-ref-is-identity-not-content]]", "[[invariants-enforced-by-the-substrate]]"]
---

# The type is not in the identity, so the gate refuses a facet changing type

**Decision (Matt, 2026-10-10, option A; built 2026-10-13, card 105).** A facet's identity stays
source plus identity values, without the type. Each importer keeps its types' identities apart, and
the gate refuses an envelope whose facet id is already held by a facet of another type.

## The tension

A facet's id is UUIDv5 of its source_ref, and source_ref is the source and the identity values
([[source-ref-is-identity-not-content]]). Building the Garmin importer (card 55) turned up a quiet
failure. Under source `garmin`, `garmin.day` with id `2026-10-09` came back `superseded` against
`health.step-count` with the same id. The gate had treated a new type as a new version of the old
fact, so the step count was overwritten by a day summary. Nothing errored.

Two fixes were on the table. Option B put the type into every identity, which closes the hole by
construction. It would also have re-keyed every live fact, moving the 5,531 Withings and 1,317
DreamStation facets to new ids. Option A leaves identity alone, has importers prefix values that
could collide (`sleep:`, `steps:`, `day:`), and has the gate make any collision impossible to miss.
Matt chose A.

The general lesson is that a dimension left out of an identity needs a check at admission. Without
one, two facts that differ only in that dimension become one fact with a history.

## The rule

- The gate compares the current version's type with the envelope's. Another type is
  `ErrTypeChanged`, naming the source_ref, both types and the current version.
- Offenders are gathered into the same refusal the other pillars use, so a batch with one is refused
  whole (422 over the API), and W does not move. The check runs before anything is written.
- `docs/IMPORTERS.md` §8.5 states the rule for importers: identity is source and identity values, so
  an importer keeps its types' identities apart. ARCHITECTURE §6a says the type is not in the
  identity and why the gate refuses.
