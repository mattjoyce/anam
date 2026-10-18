---
id: 202610171800
type: decision
title: "Handling is where to look, not a vocabulary Anam enforces"
created: 2026-10-17
status: budding
tags: [anam, decision, definitions, importers, meds, guidance]
project: anam
related: ["[[facet-definition-format]]", "[[identity-is-definition-owned]]", "[[one-owner-per-definition]]", "[[doctrine-aphorisms-outlive-their-reasoning]]"]
---

# Handling is where to look, not a vocabulary Anam enforces

A Definition has a third block, `handling`, beside `meta` and `schema`. It carries what a writer
or reader of the type needs to know that the schema cannot say: how to normalise a drug name, the
shape of a self-log id, how to say a date is estimated, what makes a file reference stale. Anam
stores it, serves it verbatim and refuses a change to it without a version bump. It does not
interpret it.

Matt decided this on 17 October (card 166), after ThinkPad.anam-157 wrote his supplements as
`med.*` records with no guidance and guessed three conventions, each passing the admission test
and each able to split one regimen into two series if another writer guessed differently.

## The tension

The first draft was a closed vocabulary of verbs, failing fast on an unknown one, read at runtime
by the importer base so an importer could not ignore it. That gives enforcement and costs the
thing the block is for: guidance is prose about judgement, and a vocabulary admits only what its
author foresaw. A loose block can be read by a person or a model writing records by hand, which is
the writer that needed it. Enforcement that matters lives where it already is, in the schema and
the identity key; a guidance line is for what those cannot hold.

## Consequences

- The block is loose JSON. Present, it must be an object; absent is fine.
- `kind` and `family`, which the inspector draws from, are `meta` fields: what a type is, not how
  it is handled. Shape and colour stay with the page.
- The card is a parser change, a 409 on a changed block, two read surfaces and a paragraph of docs.
  The first block goes on `med.started` v5.
- Status is budding: the build is pending Matt's approval of that first guidance text, and the
  shape may move when `doc.link` (card 165) needs rules per identity mode.

## Rejected

- **A closed vocabulary with fail-fast.** Enforces what cannot be foreseen and excludes the
  hand-writer.
- **Guidance in the schema's descriptions.** Descriptions are per field; the conventions that bit
  were across fields and across records.
- **Names `conduct` and `rules`.** `conduct` reads as the importer's behaviour, `rules` promises
  enforcement; `handling` says how the type is handled.
