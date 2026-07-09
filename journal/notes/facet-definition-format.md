---
id: 202607100006
type: decision
title: "Facet Definition format — JSON manifest, borrowed schema subset + governance DSL"
created: 2026-07-10
status: evergreen
tags: [anam, decision, facet, schema, definition]
project: anam
related: ["[[facet-types-dotted-namespaced]]", "[[verbatim-payload-metadata-separation]]", "[[reference-implementation-mint-cli]]"]
---

# Facet Definition format — JSON manifest, borrowed schema subset + governance DSL

**Decision.** A Facet Definition is a JSON manifest (one file per type, bundled via `//go:embed`,
overridable with `anam def load`) in **two halves**:

- **`meta`** — an Anam-specific governance DSL: `facet_type`, `definition_version`, `classification`,
  `validation_mode` (`strict`|`lenient`|`verbatim`), `source_schema`, `time_semantics`
  (event/recorded-time **JSON Pointers** + parse `layout`), `identity_key`, `min_binding_assurance`.
- **`schema`** — a **curated subset of JSON Schema** vocabulary: `required`, `properties`
  (`pattern`, `enum`), `additionalProperties`. Nothing more.

## The question

Light bespoke DSL, or borrow a schema markup standard?

## Why neither extreme

- **Not a bespoke DSL** — inventing grammar is a maintenance tax and a Hickey *complect*: a new language
  braided into the project for no gain. Borrowing JSON Schema keywords means the payload half reads like
  something everyone already knows.
- **Not full JSON Schema / not openEHR archetypes** — Arch explicitly leaves an external archetype model
  **open and deferred** (Arch §15). More decisively:
  [[verbatim-payload-metadata-separation|metadata/payload separation]] means the payload is inspected
  **once**, at the gate, for a few things (identity + time fields exist; a couple of light constraints
  hold), then held **verbatim and opaque**. Running a full schema validator over a body we've promised
  not to normalise would fight the core principle. Borrow only the slice that earns its keep.

## Prior art and the one adaptation

This is exactly what [[reference-implementation-mint-cli|`mint-cli`]] proved (`meta` + JSON-Schema-subset
+ `validation_mode` + `identity_key`). The one change: mint-cli assumes **flat string maps**; Anam's
health payloads are **nested** (`{period_start, metrics:{sleep_total:{avg,min,max}}}`), so definitions
address fields by **JSON Pointer** (`/period_start`, `/metrics/sleep_total/avg`) — one notch more
general, same idea.

## `validation_mode` is the pressure valve

`strict` (stable known columns) · `lenient` (source may grow fields — the BP CSV already carries
weight/SpO₂/glucose sporadically) · `verbatim` (opaque; only identity + time required). One mechanism
serves both tidy and messy sources.

## Open thread

Whether `identity_key` belongs in `meta` or should be derived from `source_ref`. Leaning `meta` (the
definition owns identity), with `source_ref` composed from it — but unsettled.
