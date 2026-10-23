---
type: component
title: Definitions registry (internal/def)
description: Parses Definition manifests, holds the bundled set from embedded JSON, merges registered ones, and guards a new version against re-keying.
timestamp: 2026-10-17T00:00:00Z
resource: internal/def/register.go#Register
anchor: "func Register(body []byte, stored, registered []Stored, bundled *Registry) (Registration, error) {"
basis: code
tier: supporting
group: kernel
depends_on:
  - /components/facet.md
---

| Function | Does |
|---|---|
| `Bundled` | the registry of the 31 manifests under `internal/def/manifests`, embedded in the binary |
| `Parse` | one [Definition manifest](/data-structures/definition-manifest.md) from bytes, refusing an open schema |
| `Guard` | at daemon start (`guardBundled`), refuses a stored version whose `identity_key`, `identity_template` or `fact_time` differs from the bundled one, but for two widenings proved from the stored versions (card 198): a `zone_pointer` added to the fact time, and optional placeholders appended to the template |
| `Register` | decides an importer's registration: bundled type is `ErrBundled` unless it is the body the binary carries (unchanged, card 139), same body is adopted, different body at a held version is `ErrRegistered`, older version is `ErrVersionBehind`, re-keyed is `ErrKeyed` |
| `WithRegistered` | the live registry: bundled first, registered rows for types the binary does not carry |
| `KeyingOf`, `Definition.SameIdentity` | the keying (`identity_key`, `fact_time`) read from a stored body as Guard reads it, and whether a Definition composes ids from the same identity key; what a rename compares across two types, since a different key would give different ids (card 140). A rename may change the fact-time rule (card 195) |
| `SetVersion` | a hash over the bundled manifests, pinned into receipts |
| `Registry.Closes` | whether a type closes its series, for the snapshot read |

A type lives in one place: while the binary bundles it no importer may register it, and a registered row for a bundled type is shadowed with a log line.

Since 2026-10-13 (card 111) `manifests/history/<type>.v<n>.json` carries every earlier version a facet may have been admitted under; `Manifests()` returns them with the current bodies so the start-up upsert provisions the whole history into the operational store, while `Bundled()` and `SetVersion()` skip them. Card 157 found that `note.annotation` v2 had been bumped without copying v1 there (the type's lineage: `note.annotation` (v1 to v3), then `record.note` (v1, v2), now `record.edge`), so a rebuilt operational store would have lost the words the first notes were admitted under; v1 and v2 now both ship in history. A history body is words, not a contract: checked to be JSON naming its type and version, never parsed as one.

Since 2026-10-24 (card 199) a Definition may declare `handling.label`, a short template of literal text and `{/pointer}` fields that a page draws a facet of the type as (`label.go`). It is the one key of `handling` Anam reads. `Parse` checks it (a non-empty string, braces closed, at least one field, no `{source}`, transform or optional mark, every pointer a field the schema declares) and `Definition.Label(payload)` renders it: a missing or non-scalar field is nothing, a text field is its first line, spaces collapse, and a label whose fields are all empty is `""`, so the page falls back to the type and the time. A label is part of the version, so `health.weight` v6, `health.medication.dose` v3, `mind.attention` v2, `health.intake` v2 and `record.edge` v3 are the versions that added one; a type whose importer shapes envelopes to a fixed version (blood pressure) waits for the importer.

Since card 198 a fact may name its own zone: `time_semantics.fact_time.zone_pointer`, a field of `source_time`, is read in place of the Definition's zone when the payload holds it (`factzone.go`), and the gate refuses an offset the named IANA zone does not keep at that wall clock. `source_time` has a second shape, `sourcetime/v2.json`, which adds and requires `offset`. An identity template placeholder may end in `?`, rendering nothing when its pointer is absent. `record.edge` v2 and `health.medication.dose` v2 require `source_time` and read its offset; `health.weight` v5 declares it, since records sends weight without it.
