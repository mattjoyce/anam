---
id: 202607102350
type: decision
title: "A tombstone is a record-event, not an artifact version"
created: 2026-07-10
status: evergreen
tags: [anam, decision, tombstone, status, fold, arch]
project: anam
related: ["[[invariants-as-structural-tests]]", "[[doctrine-aphorisms-outlive-their-reasoning]]", "[[ledger-and-canonical-two-surfaces]]"]
---

# A tombstone is a record-event, not an artifact version

**Decision.** Retraction appends a `tombstoned` record-event against the current artifact version.
No new artifact is written. `Mark` — event + watermark bump — is the whole mechanism.

## The tension

`ARCHITECTURE.md` §6a and the glossary both said the opposite: *"a status-bearing artifact version
carrying `tombstoned`"*. That is **Arch's** sentence (ADR 0030), and in Arch it is true — an Arch
artifact stores its current status, so retraction must write a new version to carry the new status.

Anam folded status out of the artifact at R0. `Facet` has no `Status` field; a reflection test fails the
build if one appears; a Facet has no status field at all. **There is nowhere on an Anam
artifact to put `tombstoned`.** The doc and the type system contradicted each other for the project's
entire life, and nothing noticed — because no code had yet needed to write a tombstone. R2's append
primitive was the first thing that had to know which was true.

## What it settled downstream

The ledger's write surface became **two calls, not one**: `Admit(facet, event)` for artifacts with their
events, `Mark(event)` for status transitions that carry no artifact. An `Append` with an optional facet
pointer would have made `{no artifact, admitted}` representable — the same illegal-state shape already
flagged in `Provenance`.

## The lesson

The type system had the answer before the prose did. When doctrine inherited from a parent system meets
a domain type that refuses it, believe the type — it was built under this system's invariants, not the
ancestor's. See [[doctrine-aphorisms-outlive-their-reasoning]].
