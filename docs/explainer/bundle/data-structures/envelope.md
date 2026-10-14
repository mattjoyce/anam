---
type: data-structure
title: Envelope
description: The one message that enters the gate; the Go struct the daemon builds from a submitted envelope, with source and injector set from the token.
timestamp: 2026-10-11T00:00:00Z
resource: internal/facet/facet.go#Envelope
anchor: "type Envelope struct {"
basis: code
tier: supporting
group: wire
references:
  - to: /data-structures/definition-manifest.md
    how: shaped to
    card: "n:1"
---

| Field | Type | Set from | Checked by |
|---|---|---|---|
| `Type` | facet.Type | the submitted envelope | dotted namespaced regexp |
| `Source` | string | the importer's source, named by its token | non-empty |
| `Injector` | string | same as source | non-empty |
| `SourceRef` | string | optional | the gate composes it, or refuses a mismatch |
| `Payload` | []byte | raw JSON | non-empty, at most 300 KiB, canonicalisable, passes the schema |
| `DefinitionVersion` | int | required at the daemon | must equal the type's current version; the gate alone accepts 0 as unstated |
| `AssertedPerson` | string | optional | drives binding assurance |
| `SpanHint` | string | optional | advisory, copied to the binding |

On the wire it is the [batch submission](/data-structures/batch-submission.md)'s element; the importer builds it with `anam_importer.envelope`, which names no source and no source ref.
