---
id: 202610101400
type: decision
title: "One owner per Definition"
created: 2026-10-10
status: evergreen
tags: [anam, decision, definitions, importers, versioning, deploy]
project: anam
related: ["[[identity-is-definition-owned]]", "[[sources-reach-anam-only-through-importers]]", "[[facet-definition-format]]"]
---

# One owner per Definition

**Decision (Matt, 2026-10-10).** A Facet Definition has exactly one place where a change to it is
made. While a type is both bundled in Anam and shipped by an importer, the core stream does not bump
it. Once the importer is live, the type leaves Anam's bundle for the importer's folder:
`health.measurement` with the Withings importer (card 86), `health.blood-pressure` with the blood
pressure importer (card 80). Recorded on card 50's handoffs and card 86.

## The tension

Card 71 described every bundled Definition, and documenting `health.blood-pressure` bumped it to v3.
The gate admits an envelope only at a type's current version, so the importer's own copy at v2 was
refused. The words had changed and the contract had not, and still both sides had to ship together.

Three ways out were on the table:

- **Leave it.** Every documentation edit becomes a coordinated deploy across two repos.
- **Stop versioning the words.** That loses "a fact reads under the words it was admitted with",
  which is why descriptions live in the Definition at all.
- **Teach the gate equivalence.** Two versions with the same contract minus annotations would admit
  alike. That puts a rule into the error kernel to tolerate a duplication meant to be transitional,
  and the rule would outlive the duplication.

The chosen fix removes the cause. The joint deploy was not a fault in the gate. It was duplicated
ownership showing up as deploy order.

## The rule

- A bundled type an importer also ships is frozen in the bundle until the importer takes it.
- Once the importer is live, the type leaves the bundle. The importer registers its copy at the same
  version on its next batch, because every batch carries its manifests and checks them against what
  Anam publishes. Nobody sequences the cut-over. On 10 October `health.measurement` read 404 until
  one Withings poll, the importer registered its byte-identical copy at v3, and 5,531 facets resent
  as no-op.
- The test for any shared schema is to ask where a change to it is made. Two answers mean the
  coupling is already there.

Standard health types later came back into the bundle as core's own (types name the concept, the
brand is the source), and an importer may share a bundled type it does not own (card 122). The rule
held through that. Each type still has one owner, and the other side only uses it.
