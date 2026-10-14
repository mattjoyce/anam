---
id: 202610082242
type: decision
title: "Sources reach Anam only through importers, over the envelope API"
created: 2026-10-08
status: evergreen
tags: [anam, decision, importers, gate, envelope, ductile]
project: anam
related: ["[[substrate-is-accidental]]", "[[guiding-lens-hickey-armstrong]]", "[[identity-is-definition-owned]]", "[[one-owner-per-definition]]"]
---

# Sources reach Anam only through importers, over the envelope API

**Decision (Matt, 2026-10-08, built 2026-10-09).** Anam knows no vendor. Each source is read by an
importer, a ductile plugin that shapes one source's data to a Facet Definition and posts envelopes to
the service under its own token. Anam keeps the Definitions, the gate and the record. Matt adopted
the proposal in `docs/IMPORTERS.md` on 8 October ("I think we have our evolution", card 50) and
agreed its order one rung at a time. Card 53 opened the door on 9 October with
`POST /v1/envelopes` and `POST /v1/definitions`.

## The tension

Compiled-in capture workers were easy. One `anam ingest` rescanned every source, a dead share stopped
the whole run before anything was written, and nothing crossed a network. But the binary that owns
the ledger carried vendor table layouts, and sharing a binary made the drift from the `records`
worker free. That worker already asked the Definition how to compose identity and knew no source.
ARCHITECTURE.md §2 already called capture "a pluggable client, not part of the kernel", so the code
disagreed with the design.

The lineage made the choice sharper. Senechal was cleaved into ductile (when to do things) and
`healthdata.db` (what was measured), and Anam succeeded the latter. Each cleave left a braid behind.
Moving the importers out removes the one on Anam's side. It also names two drift checks: ductile
holding schemas or derived data is Senechal growing back there, and Anam scheduling, polling or
knowing a vendor's tables is the same here.

What it costs was written down before it was chosen. New Definitions where a source's real grain
differs from the ETL's summaries. About 5,800 `health-db` facets retracted as their source goes. Each
importer needs a "send everything again" mode, since no single command rescans the world. And each
source now succeeds or fails on its own.

## The rule

- **The envelope is the only ingress.** A batch of up to 49, admitted whole or refused whole with
  every failing envelope named. An importer's token opens only the importer routes, and only for the
  types it declares in `importers_file`.
- **Definitions travel with the importer.** They live in the importer's folder in `anam-importers`
  and are registered from there. A registered version is immutable. Every batch carries the manifests
  it was shaped to, so registration needs no ordering.
- **An importer shapes and does not join or summarise.** Joining is the timeline. A weekly average
  is a projection, not an admitted fact.
- **Anam reads no source itself.** On 11 October Matt extended the rule to `records` and `documents`,
  the last two built-in workers, and card 119 finished it on 14 October.

The gate is the error kernel either way. Validation moved from "trusted code in the same binary" to
"external input at a boundary", which is where AGENTS.md always said it was.
