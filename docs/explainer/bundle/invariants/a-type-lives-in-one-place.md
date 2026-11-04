---
type: invariant
title: A type lives in one place
description: While the deployment's catalog holds a type no importer may register another body of it and any number may submit it (the one body the catalog holds is answered unchanged); an importer takes a type over when the catalog drops it; two importers naming an unbundled type refuse the daemon's start.
timestamp: 2026-10-20T00:00:00Z
resource: internal/def/register.go#Register
anchor: "if b, ok := bundled.Lookup(d.Type); ok {"
basis: code
tier: supporting
enforced_at: internal/config/principals.go#LoadPrincipals
enforced_anchor: "if bundled != nil && bundled(t) {"
applies_to:
  - /components/def.md
  - /components/config.md
  - /components/daemon.md
  - /data-structures/importers-file.md
tested_by:
  - internal/daemon/envelopes_test.go#TestImportersShareABundledType
  - internal/daemon/takeover_test.go#TestATypeDroppedFromTheBinaryIsTakenOverByItsImporter
  - internal/config/principals_test.go#TestLoadPrincipals
---

docs/IMPORTERS.md §8.1, refined in card 122. A catalog type's Definitions come from the deployment's catalog, so no importer can register a version that refuses another's envelopes, and each may submit it under its own source (Garmin and Concept2 both sent `activity.exercise-session` until card 195 gave each its own leaf). The bundled set is the predicate `LoadPrincipals` takes, so the ownership check moves with each change to the catalog.
