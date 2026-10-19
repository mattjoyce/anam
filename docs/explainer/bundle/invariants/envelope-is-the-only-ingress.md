---
type: invariant
title: The Envelope is the only ingress
description: Importers share nothing with the ledger except through an envelope posted to the daemon; only an importer's token opens the submit routes, and since card 119 no code in the binary reads a source.
timestamp: 2026-10-17T00:00:00Z
resource: internal/daemon/daemon.go#Handler
anchor: "mux.Handle(rt.pattern, d.admit(rt.roles, rt.handler))"
basis: code
tier: supporting
enforced_at: internal/daemon/api.go#routes
enforced_anchor: "f(\"POST /v1/envelopes\", importerOnly, d.postEnvelopes),"
applies_to:
  - /components/gate.md
  - /components/daemon.md
  - /components/importer-base.md
  - /components/importer-documents.md
  - /components/principals.md
  - /terms/envelope.md
---

ARCHITECTURE §12, last bullet, first half, and AGENTS.md (share-nothing). It is the HTTP contract: facts reach the record only through `POST /v1/envelopes`, and only an importer's token opens it. The route table gives the importer role `POST /v1/envelopes`, `POST /v1/definitions` and `POST /v1/documents`; the operator submits nothing, since it is no source, and a reader reads and publishes receipts. Since card 89 an importer's token also reads, as a reader's does, and sees only its own source's snapshots, so the old wording that an importer "cannot read the record" no longer holds; share-nothing is about writes (see [each route names the roles it admits](/invariants/each-route-names-the-roles-it-admits.md)).

Until card 119 the binary also held `internal/capture`, two in-process workers for `records` and `documents`, and `check.sh` failed the build if one imported `internal/store`. Both moved to importers and the package is gone, so the gate has one admitting caller, the daemon's batch route (`anam retract` uses the gate only to tombstone). A document's bytes come in beside the envelopes, through `POST /v1/documents`, but they are not a fact: the `record.document` envelope that names them is, and it is refused unless the library holds those bytes (see [hand Anam a document](/workflows/hand-anam-a-document.md)).
