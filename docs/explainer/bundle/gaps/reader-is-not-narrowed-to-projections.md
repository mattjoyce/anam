---
type: gap
title: A reader is not narrowed to projections
description: The reader role opens every read route, so a reader sees the whole record; docs/API.md calls narrowing a reader to some projections later work, and no code does it.
timestamp: 2026-10-17T00:00:00Z
looked_at:
  - docs/API.md
  - internal/daemon/api.go
  - internal/config/principals.go
---

`routes()` marks `GET /v1/timeline`, `GET /v1/changes` and the other reads `anyRole`, and a `Principal` has no field that restricts which projection or types it may read; an importer's `Types` bound what it submits, not what it reads. API.md §2 says the roles are the floor and a reader narrowed to some projections is later work, not a role. Until then a reader token is as wide as the operator's on reads, minus the write routes and the inspector. See [principals and roles](/components/principals.md).
