---
id: 202607100005
type: decision
title: "Facet types are dotted and namespaced"
created: 2026-07-10
status: evergreen
tags: [anam, decision, naming, facet]
project: anam
related: ["[[facet-definition-format]]", "[[the-inversion]]"]
---

# Facet types are dotted and namespaced

**Decision.** Facet types are named **lower-case, dotted, namespaced**: `health.sleep`,
`health.body-composition`, `health.heart-rate`, `health.blood-pressure`, `health.metric-group`. Future
sources slot in: `calendar.event`, `mail.thread`, `note.zettel`.

## The tension

Arch names facet types in **snake_case** (`clinical_note`, `blood_pressure`, `documentation_gap`). Anam
is a faithful child of Arch, so the default pull is to match. But the Ductile Naming Standard shapes
*event-type-like* names as **lower-case dotted** (`backup.archive.created`), and a facet type is
event-type-shaped — a namespaced classification of a kind of fact.

## Why dotted wins

Anam's whole premise is *"all sorts of facets"* — health first, then calendar, mail, notes, and more.
A flat snake_case space (`blood_pressure`, `calendar_event`, `mail_thread`) muddles as it grows; a
namespace (`health.*`, `calendar.*`, `mail.*`) keeps kinds sorted and lets a source own its prefix. The
namespace *earns its keep* precisely because the set is meant to expand.

## Cost

A deliberate, documented deviation from Arch. Recorded here and in `AGENTS.md` so it doesn't silently
drift back to snake_case. Payload/data keys stay snake_case (`period_start`, `systolic`); only the *type
name* is dotted.
