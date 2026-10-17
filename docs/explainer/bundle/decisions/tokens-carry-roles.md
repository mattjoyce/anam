---
type: decision
title: A token names a principal with a role
description: "Every caller is a principal (name, role, token hash); importers keep their source and types, and readers and operators get no source; a route names the roles it admits and the handlers never branch on who is calling."
timestamp: 2026-10-17T00:00:00Z
resource: internal/config/principals.go#LoadPrincipals
anchor: "func LoadPrincipals(path string, bundled func(facet.Type) bool) ([]Principal, error) {"
basis: code
tier: supporting
tested_by:
  - internal/config/principals_test.go#TestLoadPrincipalsWithRoles
  - internal/daemon/roles_test.go#TestAReaderMayNotHoldTheOperatorsToken
alternatives:
  - Keep two kinds of caller, the operator's one token and per-source importer tokens, and add a wrapper per new kind (how the daemon was until card 89; an assistant that only reads would have needed the operator's token, which also mints, backs up and opens the inspector)
  - Per-route scope lists on each token (finer than anyone needs; the roles are three sets of routes, written once in the route table)
  - A role switch inside each handler (spreads the access rule over every handler instead of one table)
affects:
  - /components/daemon.md
  - /components/principals.md
  - /data-structures/importers-file.md
  - /components/anam-submit.md
---

**Context.** Card 89. A client that reads (an assistant answering from projections) had no token between the operator's, which opens everything, and an importer's, which could read nothing and write one source.

**Choice.** A principal is a name, a role, the SHA-256 of its token and, for an importer, a source and types. `token_file` is the operator, held as a hash like the rest. The importers file keeps its `importers` key, each entry an importer named for its source, and gains a `principals` key whose entries state their name and role. Names, sources and token hashes are each used once; a principal that is not an importer has no source and no types. The decision for route access is one table (see [each route names the roles it admits](/invariants/each-route-names-the-roles-it-admits.md)).

**Why.** Roles are the floor of access control: the role says which routes a token opens. A reader reads the whole record; narrowing a reader to some projections is later work (see [a reader is not narrowed to projections](/gaps/reader-is-not-narrowed-to-projections.md)).

**What it costs.** Adding a principal restarts the daemon, as adding an importer always did. An importer now reads the whole record, which the earlier importer token could not (see [the Envelope is the only ingress](/invariants/envelope-is-the-only-ingress.md)): the narrowing that remains is that it submits as one source for its types.
