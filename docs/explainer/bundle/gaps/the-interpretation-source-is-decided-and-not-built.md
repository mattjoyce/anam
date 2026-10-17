---
type: gap
title: The interpretation source is decided and not built
description: Card 93 decided that a model's reading of a note is a second source of draft facts the person endorses; only the note half exists, as note.annotation v3, and no Definition, route or inspector view carries a standing or an endorsement.
timestamp: 2026-10-17T00:00:00Z
looked_at:
  - docs/API.md
  - internal/def/manifests/note.annotation.json
  - internal/def/manifests
---

No manifest under `internal/def/manifests` mentions an interpretation, and no Definition has a `standing` field; the vocabulary already holds `endorsed` and `rejected` event kinds, and putting them to use for drafts is the unbuilt part. Cards 158 to 161 cut the slices (draft facts, endorsement, inspector review, re-interpretation). Whether the conversation importer is live depends on the gateway registering it, which this repository does not show. See [a person's words are a note](/decisions/a-persons-words-are-a-note-and-a-models-reading-is-a-draft.md).
