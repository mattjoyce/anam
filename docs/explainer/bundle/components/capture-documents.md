---
type: component
title: Documents worker (internal/capture/documents)
description: "Reads a library folder and emits one doc.reference per file: library-relative path, sha256, size, media type and the date its sidecar gives it."
timestamp: 2026-10-11T00:00:00Z
resource: internal/capture/documents/documents.go#Scan
anchor: "func Scan(root, assertedPerson string) ([]facet.Envelope, error) {"
basis: code
tier: detail
group: sources
depends_on:
  - /components/facet.md
---

The first reference payload: the facet points at a binary in its source and Anam never stores the bytes. The root is a place and never enters a fact, so a library that moves is a config change and a file that changes supersedes. A file without a `.meta.yaml` sidecar is refused. The default library is `documents/` beside the ledger and needs no declaration.
