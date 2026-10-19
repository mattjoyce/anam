---
type: term
title: Series
description: Which facets are repeated statements about one thing, stamped at the gate so a snapshot can keep the latest per thing.
timestamp: 2026-10-11T00:00:00Z
---

A series says which facets are repeated statements about one thing over time. It is neither identity (every change is its own row) nor type (`health.medication-started` and `health.medication-stopped` are one regimen). A [Definition](/terms/facet-definition.md) declares a family, the ordered key pointers that name one series within it, and whether its type closes the series. The gate composes `<family>:<values>` and stamps it on the facet beside fact-time, so no read opens a payload to group.
