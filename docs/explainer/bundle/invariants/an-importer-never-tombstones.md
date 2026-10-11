---
type: invariant
title: An importer never tombstones; the operator retracts
description: "There is no automatic retraction: an importer reports what its source still holds through a snapshot, and only anam retract, with guards and --yes, writes a tombstone."
timestamp: 2026-10-11T00:00:00Z
resource: docs/IMPORTERS.md#Retraction by complete resend
anchor: "An importer never tombstones."
basis: docs
tier: supporting
enforced_at: not enforced
applies_to:
  - /components/anam-submit.md
  - /components/daemon.md
  - /workflows/retract-a-source.md
  - /terms/tombstone.md
---

docs/IMPORTERS.md §8.6; Matt, 2026-10-10. The daemon offers no route that tombstones, so the invariant holds by the absence of an ingress rather than by a check: `Mark` is reachable only from `gate.Retract`, which only `anam retract` and the tests call. A tombstone cannot be undone (a resend of the same row is a no-op), and an importer is outside the error kernel.
