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
| `Guard` | at daemon start (`guardBundled`), refuses a stored version whose `identity_key` or `fact_time` differs from the bundled one |
| `Register` | decides an importer's registration: bundled type is `ErrBundled` unless it is the body the binary carries (unchanged, card 139), same body is adopted, different body at a held version is `ErrRegistered`, older version is `ErrVersionBehind`, re-keyed is `ErrKeyed` |
| `WithRegistered` | the live registry: bundled first, registered rows for types the binary does not carry |
| `SetVersion` | a hash over the bundled manifests, pinned into receipts |
| `Registry.Closes` | whether a type closes its series, for the snapshot read |

A type lives in one place: while the binary bundles it no importer may register it, and a registered row for a bundled type is shadowed with a log line.

Since 2026-10-13 (card 111) `manifests/history/<type>.v<n>.json` carries every earlier version a facet may have been admitted under; `Manifests()` returns them with the current bodies so the start-up upsert provisions the whole history into the operational store, while `Bundled()` and `SetVersion()` skip them. Card 157 found that `note.annotation` v2 had been bumped without copying v1 there, so a rebuilt operational store would have lost the words the first notes were admitted under; v1 and v2 now both ship in history. A history body is words, not a contract: checked to be JSON naming its type and version, never parsed as one.
