---
type: change-path
title: Add or change a projection template
description: A template is one embedded JSON file; a changed prune is a new version, because receipts pin the version and verify refuses a moved one.
timestamp: 2026-10-11T00:00:00Z
resource: internal/projection/manifest.go#Bundled
anchor: "//go:embed manifests/*.json"
basis: code
tier: detail
touches:
  - /data-structures/projection-template.md
  - /components/projection.md
  - /components/serve.md
  - /data-structures/receipt.md
  - /invariants/projections-prune-never-reshape.md
  - /workflows/publish-a-projection.md
---

1. Add `internal/projection/manifests/<name>.json` with `name`, `version`, `types`, `statuses`, `max_classification`, `from`, `to` and optionally `shape: snapshot`. The file name must equal `name`.
2. A snapshot shape needs the types to share a series family in their Definitions, or the shape has nothing to group on.
3. Changing an existing template's filters bumps `version`. Receipts served under the old version still verify only if the old pruning can be reproduced, which it cannot once the file changed: `Verify` reports `ErrTemplateMoved`. Treat a template as immutable per version.
4. `anam serve` with no argument lists it; `anam timeline --projection <name> --explain` shows each facet's verdict; the inspector's `/projection` switches between templates.
5. The explain tests run every template over random values; add the template's name to the table in `explain_test.go` if a test enumerates them.
