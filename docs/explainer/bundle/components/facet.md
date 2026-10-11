---
type: component
title: Facet domain values (internal/facet)
description: "The value types everything else speaks in: Facet, AnamEvent, Envelope, the five event kinds, and SourceRef, the one function that composes identity."
timestamp: 2026-10-11T00:00:00Z
resource: internal/facet/id.go#SourceRef
anchor: "func SourceRef(source string, values ...string) string {"
basis: code
tier: supporting
group: kernel
depends_on: []
---

No dependencies, by design: the domain does not know what it is stored in. `SourceRef` is the source name followed by each identity value length-prefixed (`<bytes>:<value>`), so two tuples can never compose one string however many colons a value carries. `Canonicalize` gives the payload bytes the content hash is taken over. The structs are the [facet artifact](/data-structures/facet-artifact.md), the [anam-event](/data-structures/anam-event.md) and the [envelope](/data-structures/envelope.md).
