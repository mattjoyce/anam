---
type: workflow
title: Register a Definition
description: anam_submit finds Anam does not publish a Definition version the batch carries and registers it; the daemon checks it against the bundled set, the held versions and the keying rule before holding it immutably.
timestamp: 2026-10-11T00:00:00Z
resource: internal/daemon/definitions.go#postDefinitions
anchor: "parsed, err := def.Parse(body)"
basis: code
tier: supporting
trigger: The first `anam_dreamstation` batch reaches a daemon whose binary does not bundle `health.cpap-session.dreamstation`, or an operational store was lost and rebuilt, so the GET below is a 404.
steps:
  - actor: /components/importer-dreamstation.md
    caller: outside
    label: the card run carries its own Definition
    action: A folder watch on the card copy runs `handle`; every session on the card becomes an envelope of `health.cpap-session.dreamstation`, and each batch event carries the importer's `definitions/health.cpap-session.dreamstation.json` as text, since the binary does not bundle it.
    writes: [/data-structures/batch-event.md]
    resource: ../anam-importers/dreamstation_card/run.py#batch_events
    anchor: "whole = not card.skipped_files"
  - actor: /components/anam-submit.md
    caller: outside
    label: handle calls ensure_published
    action: Before submitting, every batch has its carried manifests parsed and checked against what Anam publishes. There is no "at start" for a ductile plugin, so this runs on every batch.
    reads: [/data-structures/batch-event.md]
    resource: ../anam-importers/anam_submit/run.py#handle
    anchor: "ensure_published(client, definitions)"
  - actor: /components/importer-base.md
    caller: /components/anam-submit.md
    label: GET /v1/definitions/{type}/{version}
    action: "`Client.definition` asks for exactly the version the importer carries; a 404 returns None, a 200 returns the manifest held."
    resource: ../anam-importers/anam_importer/client.py#definition
    anchor: "if status == 404:"
  - actor: /components/importer-base.md
    label: POST only on 404, stop if different
    action: On None the manifest's bytes are posted; on a held manifest that differs from the file the run stops with a non-retryable error, because the importer and Anam disagree about the contract.
    reads: [/data-structures/definition-manifest.md]
    resource: ../anam-importers/anam_importer/definitions.py#ensure_published
    anchor: "client.register(d.body)"
  - actor: /components/daemon.md
    caller: /components/importer-base.md
    label: importer token, type within its scope
    action: "`admit` lets only the importer role through to the route (any other role is a 403 naming it); `postDefinitions` bounds the body at `max_manifest_bytes` (1 MiB by default), parses the manifest (422 if not one) and refuses a type outside the importer's `types` with 403. Then it takes a turn on the write queue, waiting up to `importer_wait` (5 s)."
    reads: [/data-structures/importers-file.md]
    resource: internal/daemon/definitions.go#postDefinitions
    anchor: "if !policy.MayRegisterType(imp, parsed.Type) {"
  - actor: /components/def.md
    caller: /components/daemon.md
    label: Register checks bundled and held
    action: A type the binary bundles is `ErrBundled` (409), unless the body is the one the binary carries at that version, which is unchanged (200) so a release taking a type back does not break the importer still sending it (card 139). The same canonical body at a held version is adopted or unchanged; a different body at a held version is `ErrRegistered` (409).
    reads: [/data-structures/definition-manifest.md]
    resource: internal/def/register.go#Register
    anchor: "if b, ok := bundled.Lookup(d.Type); ok {"
  - actor: /components/def.md
    label: version forward, keying unchanged
    action: A version behind the newest held is `ErrVersionBehind`; a new version that changes `identity_key` or `time_semantics.fact_time` is `ErrKeyed`. Schema, classification and series may change freely, because a new keying rule needs a new ledger, not a new version.
    resource: internal/def/register.go#Register
    anchor: "if d.Version < newest.Version {"
  - actor: /components/daemon.md
    label: build the next registry before the row
    action: "`register` rebuilds the live registry with the new row first, so a row the daemon could not start on is never written, then swaps it in atomically after the insert."
    resource: internal/daemon/definitions.go#register
    anchor: "next, err := def.WithRegistered(d.defs.bundled, storedOf(append(registered, row)))"
  - actor: /components/store-ops.md
    caller: /components/daemon.md
    label: insert the immutable registered row
    action: "`RegisterDefinition` inserts into `registered_definition`; triggers refuse any later update or delete of it, for every writer."
    writes: [/data-structures/definition-manifest.md]
    resource: internal/store/ops/definitions.go#RegisterDefinition
    anchor: "INSERT INTO registered_definition (type, version, body, registered_at) VALUES (?, ?, ?, ?)"
  - actor: /components/daemon.md
    label: 201 registered, or 200 already held
    action: The body is `{type, version, new}` either way. The next batch's envelopes admit against the registered Definition.
    ends: true
    resource: internal/daemon/definitions.go#postDefinitions
    anchor: "status = http.StatusCreated"
---

This is the importer contract's "a type lives in one place" made mechanical. While the binary bundles a type, no importer may register it and any number may submit it; when a release drops the type, the one importer naming it takes it over by registering the same body, and two importers naming an unbundled type refuse the daemon's start.

Related: [a type lives in one place](/invariants/a-type-lives-in-one-place.md), [Definitions live in the operational store](/decisions/definitions-live-in-the-operational-store.md), [Definitions are registered on a 404, not at start](/gaps/definitions-registered-on-404-not-at-start.md).
