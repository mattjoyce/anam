---
type: workflow
title: Resend a source whole
description: anam_records sends its folder as one snapshot of the types it carries; the daemon binds each part to its envelopes, completes the run on the last part, and reports what is live but was not carried.
timestamp: 2026-10-11T00:00:00Z
resource: internal/daemon/snapshots.go#recordSnapshotPart
anchor: "rec, err := db.RecordSnapshotPart(ctx, ops.SnapshotPart{"
basis: code
tier: supporting
trigger: A folder watch on the records folder runs `anam_records handle`, which is `poll`; every record in the folder is sent again, labelled as the parts of one snapshot.
steps:
  - actor: /components/importer-records.md
    caller: outside
    label: read every records file with Go's rules
    action: All files are read before anything is sent. Plain scalars resolve as yaml.v3 does (`yes` stays text, `0777` is 511, `1_000` is 1000), a quoted scalar is always text, and a number Python would write differently from Go stops the run naming the record. Two records claiming one identity are refused.
    resource: ../anam-importers/records/run.py#scan
    anchor: "both declare the row"
  - actor: /components/importer-records.md
    label: one batch event per 49, current_versions
    action: "Each batch carries `definitions: []` and `current_versions: true`, because every type is bundled in Anam and a records file names no version."
    writes: [/data-structures/batch-event.md]
    resource: ../anam-importers/records/run.py#poll
    anchor: '"current_versions": True'
  - actor: /components/importer-base.md
    caller: /components/importer-records.md
    label: label the batches as parts of one run
    action: "`snapshot_parts` makes `{id: run-<uuid4>, part: i, parts: n, types: [...]}` over the types the envelopes carry; a type whose every record was removed is not in the run, so it is not reported absent."
    resource: ../anam-importers/anam_importer/envelopes.py#snapshot_parts
    anchor: 'sid = run_id or f"run-{uuid.uuid4()}"'
  - actor: /components/anam-submit.md
    caller: outside
    label: stamp the current version per envelope
    action: "`stamp_current` GETs `/v1/definitions` and sets `definition_version` on every envelope that lacks one, to the version Anam publishes as current, which is what Anam's own records worker did before card 119."
    reads: [/data-structures/batch-event.md]
    resource: ../anam-importers/anam_submit/run.py#stamp_current
    anchor: 'current = {d["type"]: d["current"] for d in client.definitions()}'
  - actor: /components/importer-base.md
    caller: /components/anam-submit.md
    label: POST each part with its snapshot label
    action: The submission carries `snapshot` beside `envelopes`; parts may go in any order and be resent.
    writes: [/data-structures/batch-submission.md]
    resource: ../anam-importers/anam_importer/client.py#submit
    anchor: 'status, body = self._call("POST", "/v1/envelopes", payload)'
  - actor: /components/daemon.md
    caller: /components/importer-base.md
    label: validate the label, digest the part
    action: The id must match `^[A-Za-z0-9._:-]{1,128}$`, parts 1 to `max_snapshot_parts` (100000 by default), every type within the importer's and every envelope's type among them. The part's digest is the sha256 over its sorted envelope lines, computed before the queue so a changed part is refused before the ledger is touched.
    reads: [/data-structures/batch-submission.md]
    resource: internal/daemon/snapshots.go#validSnapshot
    anchor: "case !snapshotID.MatchString(s.ID):"
  - actor: /components/store-ops.md
    caller: /components/daemon.md
    label: record the part after the ledger commit
    action: "In one ops transaction after admission: drop incomplete runs older than a day, insert the run on its first part with `started_w` the head after it, refuse a mismatched source, parts or types (409) or a part whose digest changed (409), insert the part and every carried facet id, and on the last part set `completed_w` and keep the source's two newest complete runs."
    writes: [/data-structures/snapshot-part.md]
    resource: internal/store/ops/snapshot.go#RecordSnapshotPart
    anchor: "if admitted != p.Parts {"
  - actor: /components/anam-submit.md
    label: on the completing part, read the report
    action: "The answer's `snapshot.complete` is true, so `handle` GETs `/v1/snapshots/<id>`: live, absent, carried and up to `max_absent_ids` (1,000 by default) absent ids, judged against `started_w` so a facet an incremental run sent beside the resend is never absent."
    reads: [/data-structures/admission-response.md, /data-structures/snapshot-part.md]
    resource: ../anam-importers/anam_submit/run.py#handle
    anchor: 'report = client.snapshot(str(snapshot["id"]))'
  - actor: /components/anam-submit.md
    label: raise the absent alert, retract nothing
    action: With `absent` above zero the event carries the counts and the first 20 ids, keyed on the snapshot id so a resent part does not alert twice. The operator then runs `anam retract records` (see [retract from a snapshot](/workflows/retract-a-source.md)).
    ends: true
    resource: ../anam-importers/anam_submit/run.py#absent_event
    anchor: "ABSENT_IDS_SHOWN = 20"
---

The resend is the rebuild's first step and the only way Anam learns what a push source no longer holds. It is idempotent by construction: every record Anam's own worker admitted before card 119 came back as no-op, which was the cut-over test for the records importer.

Related: [retraction by complete resend](/decisions/retraction-by-complete-resend.md), [an importer never tombstones](/invariants/an-importer-never-tombstones.md).
