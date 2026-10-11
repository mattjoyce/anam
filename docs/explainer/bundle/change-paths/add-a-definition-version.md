---
type: change-path
title: Change a Definition's shape with a new version
description: A new version may change the schema, classification or series but never identity_key or fact_time; bundled types change in the binary, registered ones in the importer's folder.
timestamp: 2026-10-11T00:00:00Z
resource: internal/def/guard.go#Guard
anchor: "func Guard(stored []Stored, bundled *Registry) error {"
basis: code
tier: supporting
touches:
  - /data-structures/definition-manifest.md
  - /components/def.md
  - /decisions/definitions-live-in-the-operational-store.md
  - /invariants/source-ref-is-identity-content-hash-is-content.md
  - /components/gate.md
  - /workflows/register-a-definition.md
  - /components/anam-submit.md
---

1. **Bump `definition_version`** in the manifest. The gate refuses an envelope shaped to any other version, so every importer of the type must move with it; an importer that still carries the old version will have its batches refused with `ErrDefinitionVersion`.
2. **Keep the keying.** `identity_key` and `time_semantics.fact_time` must be identical; `def.Guard` refuses the binary at startup and `def.Register` refuses an importer with `ErrKeyed`. A new placing or keying rule is a new ledger from the sources.
3. **Bundled type.** Edit `internal/def/manifests/<type>.json`; the embedded set and `def.SetVersion` change with the build. Facets admitted under the old version stay interpretable because every version is held in the `definition` table once an ingest has run the guard. Add the version's semantics to the type's test under `internal/def`.
4. **Registered type.** Edit `<importer>/definitions/<type>.json`; the next batch GETs the new version, gets a 404, and registers it (201). A different body at an already held version is a 409 and stops the importer.
5. **Existing facets are not re-shaped.** Admission is idempotent on content, so a Definition that changes its series key reaches only facets admitted after the change; re-send the source if the old rows must carry the new shape, which supersedes them.
6. **Records.** A records file names no version; `anam_records` stamps the current one, so regenerate the file from its extractor with the same ids and resend.
