---
type: term
title: Source
description: A capture origin, named lower-case hyphenated; part of every facet's identity and provenance.
timestamp: 2026-10-11T00:00:00Z
---

A source is where facets come from: `bp-csv`, `withings`, `garmin`, `records`, `documents`. It is the identity that appears in provenance, in cursors, and inside the [source ref](/terms/source-ref.md), so an [importer](/terms/importer.md) that takes over a built-in worker's facets must use the worker's source name or it would mint new facets beside the old ones. Over the daemon the source is not an envelope field; the importer's token names it.
