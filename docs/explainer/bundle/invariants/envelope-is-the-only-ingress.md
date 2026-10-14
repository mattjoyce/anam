---
type: invariant
title: The Envelope is the only ingress
description: Importers share nothing with the ledger except through an envelope posted to the daemon; an importer token opens the write routes and nothing that reads the record, and since card 119 no code in the binary reads a source.
timestamp: 2026-10-14T00:00:00Z
resource: internal/daemon/daemon.go#Handler
anchor: "// opens these and nothing else."
basis: code
tier: supporting
enforced_at: internal/daemon/importers.go#asImporter
enforced_anchor: "func (d *Daemon) asImporter(next http.HandlerFunc) http.Handler {"
applies_to:
  - /components/gate.md
  - /components/daemon.md
  - /components/importer-base.md
  - /components/importer-documents.md
  - /terms/envelope.md
---

ARCHITECTURE §12, last bullet, first half, and AGENTS.md (share-nothing). It is the HTTP contract: an importer reaches the record only through `POST /v1/envelopes`, and its token cannot read it. The daemon's handler gives an importer token four routes, `POST /v1/envelopes`, `POST /v1/definitions`, `POST /v1/documents` and its own `GET /v1/snapshots/{id}`, plus the published Definitions; everything else sits behind the operator's token.

Until card 119 the binary also held `internal/capture`, two in-process workers for `records` and `documents`, and `check.sh` failed the build if one imported `internal/store`. Both moved to importers and the package is gone, so the gate has one admitting caller, the daemon's batch route (`anam retract` uses the gate only to tombstone). A document's bytes come in beside the envelopes, through `POST /v1/documents`, but they are not a fact: the `doc.reference` envelope that names them is, and it is refused unless the library holds those bytes (see [hand Anam a document](/workflows/hand-anam-a-document.md)).
