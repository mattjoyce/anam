---
type: component
title: Config (internal/config)
description: Viper over YAML and ANAM_* env vars; validates every path and limit, refuses an unknown key or bucket width at load, and reads the importers file as principals.
timestamp: 2026-10-17T00:00:00Z
resource: internal/config/config.go#LoadOverriding
anchor: "func LoadOverriding(path string, set map[string]string) (Config, error) {"
basis: code
tier: supporting
group: tools
depends_on:
  - /components/span.md
  - /components/facet.md
---

The keys are the [config keys](/data-structures/config-keys.md). `person_id` has no default on purpose: it is pinned into the ledger at init. `Validate` calls `span.New` so a misspelt width fails at startup, not on the admission path. `LoadPrincipals` reads the [importers file](/data-structures/importers-file.md) and refuses a name, a source or a token hash used twice, a role it does not know, and two importers naming a type the binary does not bundle (see [principals and roles](/components/principals.md)). Since card 151 `Config` squashes in [`Limits`](/data-structures/limits.md), 21 bounds and waits with a `Default…` constant each, range-checked by `Limits.Validate` at load; `Load` and `LoadOverriding` set them with `setLimitDefaults`, so any of them can be overridden by a key or an `ANAM_` variable (see [a hard-coded value is a default or an invariant](/decisions/a-hard-coded-value-is-a-default-or-an-invariant.md)). The imports file and `ResolveImports` went with the in-process workers (card 119).
