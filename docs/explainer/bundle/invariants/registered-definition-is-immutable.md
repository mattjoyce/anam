---
type: invariant
title: A registered Definition version is immutable
description: Once held, a version's body never changes; the operational store's triggers refuse update and delete, and a different body at a held version is a 409.
timestamp: 2026-10-11T00:00:00Z
resource: internal/store/ops/ops.sql#registered_definition_no_update
anchor: "registered_definition is immutable"
basis: code
tier: detail
enforced_at: internal/store/ops/ops.sql#registered_definition_no_update
enforced_anchor: "registered_definition is immutable"
applies_to:
  - /components/store-ops.md
  - /components/def.md
  - /data-structures/definition-manifest.md
tested_by:
  - internal/store/ops/ops_test.go#TestRegisteredDefinitions_AreImmutable
---

docs/IMPORTERS.md §8.3, as in a schema registry. "Same body" compares canonical JSON, so reformatting a file is harmless; a change is a new version. The importer side checks the same thing with dict equality before posting and stops the run on a disagreement.
