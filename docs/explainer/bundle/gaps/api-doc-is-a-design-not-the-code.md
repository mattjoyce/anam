---
type: gap
title: docs/API.md describes a conversational API that is mostly unbuilt
description: API.md (card 72) designs DescribeRecord, ListProjections, SubmitAssertion, CorrectAssertion and more as the contract for agents; the daemon's routes are the ones in this bundle, with their roles.
timestamp: 2026-10-17T00:00:00Z
looked_at:
  - docs/API.md
  - internal/daemon/daemon.go
  - internal/daemon/api.go
---

The built routes are the ones `internal/daemon/openapi.yaml` describes, with their roles in `api.go`; `/v1/ingest` and `/v1/operations/{id}` went in card 119. Card 246 (2026-10-25) corrected API.md's opening, which said none of it was implemented, its list of what runs today and its sentence on an ingest operation's `mint`, and made `anam daemon --help` list every route. API.md's common contracts (`record_position` as `{ledger_id, watermark}`, receipted reads at a W) are built in card 87; the writing side (assertions with submission ids, corrections, managed source records) is not. API.md §2 on principals and roles (card 89) and §6 on notes from a conversation (cards 93, 157) describe what is built and decided; the surface around them is still design. A reader should take API.md as the direction, not the surface.
