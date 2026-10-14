---
type: term
title: Source
description: A capture origin, named lower-case hyphenated; part of every facet's identity and provenance.
timestamp: 2026-10-11T00:00:00Z
---

A source is where facets come from: `bp-csv`, `withings`, `garmin`, `records`, `documents`. It is the identity that appears in provenance, in cursors, and inside the [source ref](/terms/source-ref.md), so an [importer](/terms/importer.md) that takes over another's facets must use that source's name or it would mint new facets beside the old ones; `anam_records` and `anam_documents` kept `records` and `documents` when Anam's own workers went (card 119). Over the daemon the source is not an envelope field; the importer's token names it.
