---
type: gap
title: The anam-importers checkout is on card-58-one-off and behind its origin
description: The working tree is on card-58-one-off and 34 commits behind origin/master; garmin, concept2, the sha256 document upload and the conformance tests exist only on origin/master, so this explainer cites what is tracked here.
timestamp: 2026-10-17T00:00:00Z
looked_at:
  - ../anam-importers (git branch, git log --all)
  - ../anam-importers/CHANGELOG.md
  - ../anam-importers/README.md
---

On 2026-10-17 the tree is at 3451d3a on `card-58-one-off` (the Apache-2.0 notice of card 143 on top of 02a4907), branched from 236c582 before the garmin merge, and `git rev-list` counts 34 commits on origin/master that it lacks; `garmin/__pycache__` is a leftover, not tracked. Merging the branch must reconcile CHANGELOG.md and README.md, which each carry Unreleased entries the other lacks. The live Garmin importer that replaced health-db (card 55) is therefore not described here beyond the docs' account. The same holds for `anam_documents` (anam card 103, merged at 1dba240), which card 119 depends on: [its page](/components/importer-documents.md) cites Anam's docs, not the plugin.
