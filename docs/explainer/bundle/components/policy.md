---
type: component
title: Policy (internal/policy)
description: "Pure functions over the request and the importers file's principals that answer every who-may question the daemon, the inspector and the CLI ask; it reads no payload."
timestamp: 2026-10-23T00:00:00Z
resource: internal/policy/policy.go#Allow
anchor: "func Allow(req Request) Answer {"
basis: code
tier: supporting
group: entry
depends_on:
  - /components/config.md
---

Card 223. `Allow` takes a route's roles from the daemon's route table and answers allowed, unknown (401) or forbidden (403). `ScopesOf` is an importer's authenticated pillar, handed to the gate, and `MaySubmitType` asks the same scopes for a snapshot's declared types and the Workbench's allow-list. `MayRegisterType` and `MayStoreDocument` are one test: the principal lists the type. `Records`, `ReaderOf` and `TimelineRead` say how a recording token's reads are filed; `MayOpenReceipt` gates opening and verifying a receipt; `ListScope` says which receipts and snapshots a principal's lists show, as rows a per-principal override would join. `Declared`, `MayRetract` and `MayRetire` read the importers file for the sources it declares; whether a declared source has completed a snapshot is the store's part and stays in `internal/retract`. `Held` answers the admission hold: the daemon reads the hold rows inside the write and passes them in on the request, and `Holdable` and `MayHold` keep the workbench out of every hold. The package imports only `config` and `facet`, which a test holds. See [every who-may rule is one static policy package](/decisions/who-may-is-one-static-policy-package.md).
