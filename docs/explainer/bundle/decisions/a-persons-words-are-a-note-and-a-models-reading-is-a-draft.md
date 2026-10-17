---
type: decision
title: A person's words are a note, and a model's reading is a draft
description: "A conversation is an importer; the person's words are saved at once and verbatim as a note.annotation under source conversation, which may name no facet yet; a model's reading is a second source whose facts are drafts the person endorses by a new version."
timestamp: 2026-10-17T00:00:00Z
resource: docs/API.md#Writing
anchor: "which may name no facet yet (v3, card 157)"
basis: docs
tier: supporting
alternatives:
  - Store only the words (keeps the rule that a facet holds what its source said, and loses the structure the record is for)
  - Store only the model's facts (gains structure, and attributes the model's reading to the person)
  - Write the person's assertion to a managed records file and let the records importer admit it (the route API.md first designed; overtaken, since a conversation can post envelopes under its own token)
affects:
  - /data-structures/definition-manifest.md
  - /terms/importer.md
  - /components/gate.md
---

**Context.** Card 93, decided by Matt on 2026-10-15. Capture is where much of the value is, since three of the four stories in `docs/compelling-reason-for-Anam.md` are writes, but a facet holds what its source said.

**Choice.** The words are a `note.annotation` under source `conversation`, written at once. A model may read them into ordinary typed facts under a second source, `interpretation`; each links to the exact note version it came from and carries `standing: draft | endorsed | rejected`. The person endorses or rejects a draft with a new version, and nothing is deleted.

**What is built.** Only the first part: card 157 made `note.annotation` v3 accept a note that names no facet, so the words can land before any facet exists, and a later version of the same note can name them. The `interpretation` source, the `standing` field, endorsement and the inspector's review (cards 158 to 161) are cut but not built (see [the interpretation source is decided and not built](/gaps/the-interpretation-source-is-decided-and-not-built.md)).
