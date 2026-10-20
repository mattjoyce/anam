---
type: workflow
title: Publish a projection
description: anam serve bp prunes the canonical at the head with the bp template, writes the served value and a receipt that pins what was shown, and prints the receipt hash.
timestamp: 2026-10-11T00:00:00Z
resource: internal/serve/serve.go#Run
anchor: "t, err := d.Templates.Lookup(name)"
basis: code
tier: supporting
trigger: The operator runs `anam serve bp` locally (serve has no daemon route; the daemon's equivalent is `POST /v1/receipts` from `anam timeline --projection bp --publish`).
steps:
  - actor: /components/cli.md
    caller: outside
    label: serveCmd opens the local files
    action: "`serveCmd` uses `loadConfig`, which refuses a configured server, opens the ledger read-only, the operational store, the artifact store and the Definitions, and calls `serve.Run`; with no argument it lists the templates instead."
    resource: internal/app/serve.go#serveCmd
    anchor: "out, err := serve.Run(ctx, deps, args[0], dryRun)"
  - actor: /components/read.md
    caller: /components/cli.md
    label: assemble the serve dependencies
    action: "`ServeDeps` pairs the handles with the bundled projection templates, the consent gate (`consent.Open`, allow), the Definitions as the series rules, the bundled definition-set hash and the real clock."
    resource: internal/read/receipt.go#ServeDeps
    anchor: "Gate:                 consent.Open{},"
  - actor: /components/serve.md
    caller: /components/cli.md
    label: look up the bp template
    action: The registry of embedded manifests returns the template or names the available ones. Template `bp` is version 1, shape `log`, types `[health.blood-pressure]`, statuses admitted and endorsed, max classification Sensitive, unbounded window.
    reads: [/data-structures/projection-template.md]
    resource: internal/serve/serve.go#Run
    anchor: "t, err := d.Templates.Lookup(name)"
  - actor: /components/mint.md
    caller: /components/serve.md
    label: the canonical at the head, composed
    action: "`valueAt` asks `mint.ReadAt` for the composed value at the head watermark and falls back to `fold.ReadAt` when no sealed run is usable."
    reads: [/data-structures/checkpoint-value.md, /data-structures/anam-event.md]
    resource: internal/serve/serve.go#valueAt
    anchor: "c, ok, err := mint.ReadAt(ctx, d.Ledger, d.Checkpoints, d.Artifacts, w)"
  - actor: /components/projection.md
    caller: /components/serve.md
    label: apply the template, keep served entries
    action: "`Apply` runs `Explain` and keeps the entries whose reason is served, building a new value that hashes."
    reads: [/data-structures/fold-entry.md]
    resource: internal/projection/projection.go#Apply
    anchor: "verdicts, err := Explain(v, t, g, s)"
  - actor: /components/projection.md
    label: one verdict per entry, by the first knob
    action: Type allow-list, then status, then classification ceiling, then fact-time window; only entries that pass are shown to the consent gate; a snapshot shape would then demote displaced and closed entries. The payload is never read.
    resource: internal/projection/projection.go#verdict
    anchor: "case !t.admitsType(e.Type):"
  - actor: /components/serve.md
    label: build and validate the receipt
    action: "`Publish` takes the served bytes (the value's one serialisation), hashes them, and builds the receipt: served hash, facet ids in served order, watermark, ledger id, template name and version, definition-set version, served_at, span, and no query for a whole serve."
    writes: [/data-structures/receipt.md]
    resource: internal/serve/serve.go#Publish
    anchor: "ServedHash:           servedHash,"
  - actor: /components/store-artifact.md
    caller: /components/serve.md
    label: put the value, then the receipt
    action: Each file is written to a nonce-named temp file under `O_EXCL`, synced and renamed into `served/sha256-<hex>.json` then `receipt/sha256-<hex>.json`. Value first, so a crash leaves a value no receipt names and never a receipt without its value. A dry run returns before either.
    writes: [/data-structures/receipt.md]
    resource: internal/store/artifact/artifact.go#writeAtomic
    anchor: 'tmp := name + "." + hex.EncodeToString(nonce[:]) + ".tmp"'
  - actor: /components/cli.md
    label: print counts and the receipt hash
    action: Template and version, W, canonical and served counts with their hashes, the receipt hash and the span. `anam receipt verify <hash>` later re-derives at that watermark and re-hashes the file.
    ends: true
    resource: internal/app/serve.go#serveCmd
    anchor: '_, _ = fmt.Fprintf(w, "  receipt    %s\n", out.ReceiptHash)'
---

Publishing writes nothing to the ledger: being shown is not something Anam learned about a fact. That is why there is no `published` anam-event and why a facet's published time is derived from receipts.

There is no receipt index in SQLite. `anam receipt list` lists the artifact directory and sorts by served_at.

Related: [publication is not an anam-event](/decisions/publication-is-not-an-anam-event.md), [projections prune, never reshape](/invariants/projections-prune-never-reshape.md), [artifact writes are atomic](/decisions/artifact-writes-are-atomic.md).
