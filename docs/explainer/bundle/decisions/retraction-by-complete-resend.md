---
type: decision
title: Retraction by complete resend
description: An importer never tombstones; it labels a resend-everything run as the parts of one snapshot, Anam reports what is live but not carried, and the operator retracts with guards.
timestamp: 2026-10-11T00:00:00Z
resource: internal/daemon/snapshots.go#partDigest
anchor: 'return "sha256:" + hex.EncodeToString(h.Sum(nil))'
basis: docs
tier: supporting
tested_by:
  - internal/daemon/snapshots_test.go#TestSnapshotLeavesAloneWhatLandedWhileItRan
  - internal/daemon/snapshots_bind_test.go#TestSnapshotPartIsBoundToItsEnvelopes
  - internal/app/snapshot_retract_internal_test.go#TestRetract_PlansAnImportersSourceFromItsSnapshot
alternatives:
  - Let an importer send tombstones (refused; a tombstone cannot be undone by a resend, and an importer is outside the error kernel)
  - Let an ingest tombstone what a scan did not return (refused; a row missing from one scan and a source gone look alike from inside an envelope)
affects:
  - /components/daemon.md
  - /components/store-ops.md
  - /components/anam-submit.md
  - /workflows/retract-a-source.md
  - /terms/snapshot-resend.md
---

**Context.** Cards 78, 94, 107, 120. With push importers, Anam no longer scans a source and cannot see what it no longer holds.

**Choice.** A resend labels each batch `{id, part, parts, types}`. Anam keeps the parts and the facet ids they carried (no-ops included) in disposable tables, binds each part to the digest of its envelopes so a part resent with other contents is a 409, and once every part has arrived reports absent as live, of the run's types, not carried, and untouched since the run's first part. `anam retract <source>` plans from the newest complete snapshot, refuses an empty run, bounds a run at a tenth by default, and writes nothing without `--yes`. A source replaced under another name is retired whole with `--retired --yes --max-fraction 1`, after its declaration is removed.

**Why.** The one gap Anam cannot see is the importer's own, between reading its source and sending the first part; everything else is judged from the watermark.

**What it costs.** A crash between the ledger commit and recording the part leaves the part unrecorded, so a changed resend after such a crash is admitted as the part; the protocol accepts this. The alert reaches the operator through a ductile event; Anam sends nothing itself.
