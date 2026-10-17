---
type: term
title: Principal
description: A caller the daemon admits, with a name, a role and the SHA-256 of its token; an importer principal also has the one source it submits as and the types it may submit.
timestamp: 2026-10-17T00:00:00Z
---

Every request that reaches a route carries a principal, found by hashing the presented token and looking the hash up. The operator is the principal whose token is in `token_file`, named `operator`; every other principal is in the [importers file](/data-structures/importers-file.md). The daemon holds hashes, never tokens. A refusal or a log line names the principal, not its token. See [principals and roles](/components/principals.md) and [role](/terms/role.md).
