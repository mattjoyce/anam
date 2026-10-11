---
type: component
title: Config (internal/config)
description: Viper over YAML and ANAM_* env vars; validates every path, refuses an unknown key or bucket width at load, and reads the imports and importers files.
timestamp: 2026-10-11T00:00:00Z
resource: internal/config/config.go#LoadOverriding
anchor: "func LoadOverriding(path string, set map[string]string) (Config, error) {"
basis: code
tier: supporting
group: tools
depends_on:
  - /components/span.md
  - /components/facet.md
---

The keys are the [config keys](/data-structures/config-keys.md). `person_id` has no default on purpose: it is pinned into the ledger at init. `Validate` calls `span.New` so a misspelt width fails at startup, not on the ingest path. `LoadImporters` reads the [importers file](/data-structures/importers-file.md) and refuses two importers naming a type the binary does not bundle. `ResolveImports` finds `imports.yaml` for the built-in workers.
