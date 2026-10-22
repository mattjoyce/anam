---
type: component
title: Principals and roles (internal/daemon auth)
description: "Every caller is a principal with a name, a role (reader, importer or operator) and a token hash; admit lets a request through only when its role is one the route names, with 401 for an unknown token and 403 for a role the route does not admit."
timestamp: 2026-10-17T00:00:00Z
resource: internal/daemon/auth.go#admit
anchor: "case !slices.Contains(roles, who.Role):"
basis: code
tier: supporting
group: entry
depends_on:
  - /components/config.md
  - /components/def.md
---

Card 89. The daemon holds a map from the hex SHA-256 of a token to a `config.Principal`, built once by `loadPrincipals` at `Open`. The operator comes from `token_file` and is named `operator`, a name no other principal may take; everyone else comes from the [importers file](/data-structures/importers-file.md). The daemon never holds a client's token, only its hash, and a principal whose hash equals the operator's is refused at start.

| Role | Opens |
|---|---|
| `reader` | every read, and publishing a read, which writes artifacts and never the ledger |
| `importer` | a reader's routes, and submitting envelopes, documents and Definitions as its one source and for its types only |
| `operator` | a reader's routes, and minting, backups, projection templates and the inspector; it submits nothing, since it is no source |

`who` finds the principal for a request (a bearer token, or a basic-auth password so a browser can open the inspector). On a daemon with no `token_file`, which listens on loopback only, a request that presents no known token is the operator's. `admit` wraps each route with the roles its entry names in the route table (see [each route names the roles it admits](/invariants/each-route-names-the-roles-it-admits.md)), and puts the principal in the request context; handlers read it with `principalOf` and never switch on who a caller is. The one thing they read from it besides the name is its `receipts` setting, which is data in the importers file as the role is: a token set to `always` has its timeline reads and queries filed as receipts that name it (card 216). The 403 names the principal, its role, the route and the roles the route admits.

A token set to record every read must name a projection to read the timeline, since a receipt is what a projection served, and it opens only the receipts that name it, so no read of facets goes unrecorded; a `dry_run` query, which files nothing for any token, is the one read that does. If the receipt cannot be filed the read fails and returns nothing.

An importer sees only its own source's snapshots; a reader or the operator sees any. `/healthz` is outside the table and answers without a token.
