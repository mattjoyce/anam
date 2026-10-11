---
type: invariant
title: A type lives in one place
description: While the binary bundles a type no importer may register it and any number may submit it; an importer takes a type over when a release drops it; two importers naming an unbundled type refuse the daemon's start.
timestamp: 2026-10-11T00:00:00Z
resource: internal/def/register.go#Register
anchor: "if _, ok := bundled.Lookup(d.Type); ok {"
basis: code
tier: supporting
enforced_at: internal/config/importers.go#LoadImporters
enforced_anchor: "if bundled != nil && bundled(t) {"
applies_to:
  - /components/def.md
  - /components/config.md
  - /components/daemon.md
  - /data-structures/importers-file.md
tested_by:
  - internal/daemon/envelopes_test.go#TestImportersShareABundledType
  - internal/daemon/takeover_test.go#TestATypeDroppedFromTheBinaryIsTakenOverByItsImporter
  - internal/config/importers_test.go#TestLoadImporters
---

docs/IMPORTERS.md §8.1, refined in card 122. A bundled type's Definitions come from Anam, so no importer can register a version that refuses another's envelopes, and each may submit it under its own source (`exercise.session` from Garmin and Concept2). The bundled set is the predicate `LoadImporters` takes, so the ownership check moves with each release.
