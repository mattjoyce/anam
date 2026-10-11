---
type: component
title: Records worker (internal/capture/records)
description: The built-in reader of records files, facts written down as YAML; asks the Definition how identity is composed and knows no source.
timestamp: 2026-10-11T00:00:00Z
resource: internal/capture/records/records.go#Scan
anchor: "func Scan(path, assertedPerson string, defs Identities) ([]facet.Envelope, error) {"
basis: code
tier: detail
group: sources
depends_on:
  - /components/facet.md
  - /components/def.md
---

A records file has `document`, `extracted` and `records`; each record names its type and id and becomes one envelope of a bundled type. It is a format, not a source, which is why it stayed in Anam when the vendor workers left; the same reading now also exists as the [records importer](/components/importer-records.md), which reads YAML with Go's rules so the two agree facet for facet.
