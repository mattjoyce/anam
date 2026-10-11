---
type: gap
title: Definitions are registered on a 404, not at start
description: IMPORTERS §8.3 and §8.5 say an importer registers its Definitions every time it starts; a ductile plugin has no start, and anam_submit GETs each version on every batch and POSTs only when it is missing.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - ../anam-importers/anam_importer/definitions.py
  - ../anam-importers/anam_submit/run.py
  - docs/IMPORTERS.md
---

The effect the doc wants (a lost operational store comes back) still holds, because the 404 after the loss triggers the POST; the no-op 200 path of `POST /v1/definitions` is reached only by a race between the GET and the POST. The records importer carries no Definitions and skips local validation altogether, which §8.5 item 2 does not allow for. See [register a Definition](/workflows/register-a-definition.md).
