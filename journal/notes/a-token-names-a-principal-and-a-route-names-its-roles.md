---
id: 202610151700
type: decision
title: "A token names a principal with a role; a route names the roles it admits"
created: 2026-10-15
status: evergreen
tags: [anam, decision, api, auth, rbac, daemon]
project: anam
related: ["[[sources-reach-anam-only-through-importers]]", "[[a-persons-words-are-a-note-and-a-models-reading-is-a-draft]]", "[[a-hard-coded-value-is-a-default-an-invariant-or-presentation]]"]
---

# A token names a principal with a role; a route names the roles it admits

**Decision (Matt, 2026-10-11: "89 is really rbac tokens"; built 2026-10-15, card 89).** Every
caller is a principal: a name, a role (`reader`, `importer`, `operator`) and the SHA-256 of its
token. One route table in `internal/daemon/api.go` names the roles each route admits, and one
middleware checks it: 401 for no token or an unknown one, 403 for a role the route does not admit.

## The tension

The daemon knew two callers, each a code path of its own. The operator's token opened everything
but submitting; an importer's opened its three routes and could not read. A conversational client
fit neither: it needs to read the whole record, and to write as source `conversation`. The easy
move was a third special case, a reader token, beside the other two. Three code paths that each
decide access their own way is exactly the braid that grows a fourth.

## What was chosen

- **Role is data, route access is one value.** A principal's role sits in the importers file; a
  route's admitted roles sit in the table. No handler asks who the caller is in order to decide
  access. The one handler that still reads the principal, the snapshot report, scopes the answer to
  an importer's own source; it does not decide whether to answer.
- **The table is the only way a route is registered,** so the test that sends each role's token to
  every route in the table covers every route the daemon has, by construction.
- **An importer is a reader that also submits.** That is what lets one token serve a conversation:
  it reads the record and writes its own source's notes. The operator reads and runs the daemon but
  submits nothing, since it is no source and an envelope admitted under it would name none.
- **The operator is a principal too,** named `operator`, from `token_file`. It is held as a hash like
  the rest, so the daemon holds no token at all.
- **The importers file folds in rather than a second file.** A `principals` list sits beside the old
  `importers` list, which still reads as before, so the deployed file needed no change. The cost is
  a key, `importers_file`, that now names readers too.
- **The roles are invariants, not defaults.** Each is a set of routes the binary is built with; a
  role a person invents in the file would open nothing the code can name, so it is refused at start.

## Rejected

- **A reader token as a third case.** It answers card 89's first wording and leaves the shape that
  made it necessary.
- **A role hierarchy** (reader < importer < operator). The operator does not submit, so the roles
  are sets, not levels.
- **Narrowing a reader to projections now.** Roles are the floor of access control; mapping a role
  to projections is later work, on top of this.
