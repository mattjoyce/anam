---
id: 202610132320
type: decision
title: "A consumer selects a projection; the operator registers it, and every version stays held"
created: 2026-10-13
status: evergreen
tags: [anam, decision, projection, receipts, governance, api]
project: anam
related: ["[[published-is-a-receipt-not-a-record-event]]", "[[identity-is-definition-owned]]", "[[a-read-names-its-fields-and-gets-back-a-frame]]"]
---

# A consumer selects a projection; the operator registers it, and every version stays held

**Decision (Matt, 2026-10-11; built 2026-10-13, card 125).** Projection templates are registered like
Definitions. Bundled templates still ship in the binary as the floor. The operator registers new ones
and new versions over `POST /v1/projections`. A version is immutable, and every version ever held
stays readable.

## The tension

Templates were compiled into the binary (`bp`, `meds`, `sleep`). That tied governance, the choice of
which types a reader may see, to a code release. Two gaps showed the cost on the same day. `sleep` v1
named `health.sleep`, which only the retired ETL wrote, so it served nothing live. And a `heart`
template for a referral needed a release.

Moving templates out of the binary could have moved the power to define them to whoever reads. That
would break ARCHITECTURE §9, "a consumer selects a projection, it never defines one". So defining
moves from the binary to the operator, never to a reader. Importer tokens cannot register one.

The other pull was the place-shaped registry that existed, one template per name. `serve.Verify`
refused any version but the newest, so revising a template would orphan every receipt filed under the
old one. A receipt names `template` and `template_version`, and verify re-prunes through that exact
version. Old versions therefore have to stay, which makes the registry a set of values, as Definitions
already are.

## The rule

- Same name and version with the same body is unchanged. A different body at a held version is
  refused, and so is a version behind the newest.
- A template naming a type with no Definition in force is refused at registration, rather than
  serving nothing quietly.
- A template carries the sha256 of its canonical manifest, and a receipt carries it as
  `template_hash`, so verify checks the exact body and not only the version number.
- A bundled `bp` v1 stays held when `bp` v2 is registered over it, for the receipts that name v1.
- Registered templates live in the operational store's **authored** tier (ARCHITECTURE §8), not the
  provisioned one the card first proposed. The operator's file has no git home Anam knows, so a
  template comes back from a backup or by registering it again, and triggers make a version
  immutable.
- Another body at the version a receipt names is `ErrTemplateMoved` at verify.
