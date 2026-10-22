---
type: invariant
title: Each route names the roles it admits
description: The daemon's one route table lists, per route, the roles whose tokens open it; no handler checks who a caller is, so a route a role may not use is a 403 before any work is queued.
timestamp: 2026-10-17T00:00:00Z
resource: internal/daemon/api.go#routes
anchor: "importerOnly = []config.Role{config.RoleImporter}"
basis: code
tier: supporting
enforced_at: internal/policy/policy.go#Allow
enforced_anchor: "case !slices.Contains(req.Roles, req.Caller.Role):"
applies_to:
  - /components/daemon.md
  - /components/principals.md
  - /data-structures/importers-file.md
tested_by:
  - internal/daemon/roles_test.go#TestEveryRouteAdmitsTheRolesItNames
  - internal/daemon/roles_test.go#TestAReaderWritesNothingButReceipts
  - internal/policy/policy_test.go#TestAllow
---

Card 89. `routes()` returns every route but `/healthz` with its roles, and `Handler` mounts each behind `admit`. Reading the record and publishing a read are every role's. Submitting is an importer's alone, because an envelope is admitted as its importer's source and the operator is no source. Running the daemon is the operator's, and so is the inspector, which shows the whole record unprojected. Before card 89 this was three wrapper functions (`authorize`, `asImporter`, `asReader`) and the inspector sat under the operator's token by being mounted last; a new route had to pick one by remembering to.

`admit` hands the route's roles to `policy.Allow` (card 223), which keeps no copy of the table: the table stays the one source, mirrored in `openapi.yaml`'s `x-anam-roles`. The table's constructor takes the roles as an argument, so a route cannot be added without naming them, and the test sends a token of each role to every route in the table and checks it is admitted exactly when the route names its role. The roles are an invariant, not a config key: each is a set of routes this build has, and a role the build does not know would open nothing it can name.
