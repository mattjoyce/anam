---
type: term
title: Error kernel
description: "The small trusted core that must not corrupt: the append-only ledger plus the gate's admission invariants."
timestamp: 2026-10-11T00:00:00Z
---

Armstrong's term, adopted. Anam's error kernel is the [ledger](/terms/ledger.md) plus the [gate](/components/gate.md)'s invariants: three pillars, idempotency, immutability. `commit` writes artifact, anam-event and watermark in one transaction in one file. Everything above it, fold, mint, projections, importers, the CLI, is disposable and rebuildable from source.
