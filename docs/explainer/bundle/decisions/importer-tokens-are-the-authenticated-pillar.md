---
type: decision
title: Per-source importer tokens are the authenticated pillar
description: Each importer has its own token; the daemon holds only its SHA-256 beside its source and types; the token names the source, opens the write routes and its own snapshots, and cannot read the record.
timestamp: 2026-10-11T00:00:00Z
resource: internal/daemon/importers.go#of
anchor: "imp, ok := in[hex.EncodeToString(sum[:])]"
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
  - /components/anam-submit.md
---

**Context.** Splunk's HTTP Event Collector pins a token to its sourcetypes; the same shape makes the gate's second pillar credentials rather than a constant.

**Choice.** The importers file is read once at start. A request's bearer is hashed and looked up; the operator's own token is refused on importer routes and an importer's on operator routes. The importer's types become the gate's scopes for that batch and the only types it may register. The token lives in the gateway's vault, granted to one `anam_submit` instance.

**Why.** An importer cannot write another source's facets or read the record, and the daemon never holds a plaintext importer token.

**What it costs.** Adding an importer restarts the daemon; the `token_scope` table meant for this is unused (see [token_scope is a dead table](/gaps/token-scope-table-unused.md)).
