---
type: change-path
title: Add a source as an importer
description: A new vendor source is a folder in anam-importers with a manifest, a run.py that shapes payloads, and the Definitions it emits, plus a token line in the daemon's importers file.
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/README.md#Writing an importer
anchor: "## Writing an importer"
basis: docs
tier: supporting
touches:
  - /components/importer-base.md
  - /components/anam-submit.md
  - /data-structures/definition-manifest.md
  - /data-structures/importers-file.md
  - /components/daemon.md
  - /workflows/admit-a-batch.md
  - /workflows/register-a-definition.md
  - /invariants/a-type-lives-in-one-place.md
  - /invariants/importers-shape-only.md
---

1. **Decide the type.** If Anam bundles a Definition for it (28 under `internal/def/manifests`), the importer carries no Definition for that type and shapes to the current version; several importers may share it. Otherwise write `<folder>/definitions/<type>.json` in the manifest format, Open mHealth where a vocabulary exists, and no other importer may name it.
2. **Write the folder.** `manifest.yaml` (protocol 2, commands `handle`, `poll`, `health`, the same version as `VERSION`), `run.py` with a uv script header for `jsonschema`. `run.py` reads the source and builds payloads only: `envelope`, `check`, `batches`, one `<plugin>.batch` event per group, and `snapshot_parts` on a resend-everything run. Raise `Failure(message, retry=...)` when the source fails.
3. **Tests.** Unit tests beside the others under `tests/`, and an `ANAM_BIN` test against a real daemon that admits once and resends as no-op. `test_version.py` fails if the manifest's version disagrees.
4. **Token and scope.** `openssl rand -hex 32`; its SHA-256 goes in the daemon's importers file with the source name and types, and the daemon restarts to read it. The token goes in the gateway vault under its own name, granted to the `anam_submit_<source>` principal.
5. **Gateway.** An `anam_submit` instance with `token_secret`, a trigger (webhook, folder watch, schedule), and pipelines from the trigger to the importer and from `<plugin>.batch` to the instance. Route `anam.snapshot_absent` to a notifier.
6. **Cut-over, if it replaces a built-in worker.** Use the worker's source name so facets are the same facets; the first resend should be all no-op. Replacing a source under a new name instead ends with `anam retract <old> --retired`.
