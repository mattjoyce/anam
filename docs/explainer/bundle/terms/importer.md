---
type: term
title: Importer
description: A ductile plugin outside Anam that reads one source, shapes rows to a Definition, and submits them under its own token.
timestamp: 2026-10-11T00:00:00Z
---

An importer is a ductile plugin in the `anam-importers` repo that knows one vendor: it reads the native format, shapes rows to the [Facet Definition](/terms/facet-definition.md) it targets, validates locally, and emits batches that `anam_submit` posts to the daemon under the importer's token. It composes no identity, joins nothing, summarises nothing, and holds no token itself.

Anam keeps the Definitions, the gate and the record, and knows no vendor. The contract is docs/IMPORTERS.md §8. See [the importer base](/components/importer-base.md) and [anam_submit](/components/anam-submit.md).
