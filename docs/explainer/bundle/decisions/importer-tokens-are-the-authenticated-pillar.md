---
type: decision
title: Per-source importer tokens are the authenticated pillar
description: Each importer has its own token; the daemon holds only its SHA-256 beside its source and types; the token names the source, opens the write routes, and an importer reads as a reader does.
timestamp: 2026-10-17T00:00:00Z
resource: internal/daemon/auth.go#who
anchor: "if who, ok := d.principals[hashOf(p)]; ok {"
basis: docs
tier: supporting
tested_by:
  - internal/daemon/envelopes_test.go#TestImporterRoutesAuthorize
  - internal/daemon/envelopes_test.go#TestOpenRefusesAnImporterWithTheOperatorsToken
alternatives:
  - One static source-to-types map in the binary (how mvpScopes worked for the in-process workers until card 119; replaced for importers in card 53 because a push source needs real credentials)
  - Token rotation and multi-scope tokens (deferred, ARCHITECTURE §13)
affects:
  - /components/daemon.md
  - /components/gate.md
  - /data-structures/importers-file.md
  - /components/principals.md
  - /components/anam-submit.md
---

**Context.** Splunk's HTTP Event Collector pins a token to its sourcetypes; the same shape makes the gate's second pillar credentials rather than a constant.

**Choice.** The importers file is read once at start. A request's bearer is hashed and looked up, and the principal it finds must hold a role the route admits (see [a token names a principal with a role](/decisions/tokens-carry-roles.md)): the operator's token is a 403 on the submit routes, and an importer's is a 403 on minting, backups, projection templates and the inspector. The importer's types become the gate's scopes for that batch and the only types it may register. The token lives in the gateway's vault, granted to one `anam_submit` instance.

**Why.** An importer cannot write another source's facets, and the daemon never holds a plaintext importer token. Since card 89 an importer can read the record, as a reader does, and sees only its own source's snapshots.

**What it costs.** Adding an importer restarts the daemon; the `token_scope` table meant for this is unused (see [token_scope is a dead table](/gaps/token-scope-table-unused.md)).
