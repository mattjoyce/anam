---
type: workflow
title: Rebuild the record from its sources
description: After a re-keyed Definition, a new schema or a corrupted file, the operator backs up, moves the two stores aside, initialises new ones, and has every importer resend its source whole; each run's snapshot says when it has finished, and what only the old ledger held stays in the old file.
timestamp: 2026-10-17T00:00:00Z
resource: docs/IMPORTERS.md#8.8 Rebuilding the record from its sources
anchor: "**A rebuild of the live record.**"
basis: docs
tier: supporting
trigger: The operator decides the ledger must start again from nothing, after a drill against a scratch daemon has shown that every importer's resend reproduces the live record's facet ids, content hashes and statuses.
steps:
  - actor: /components/backup.md
    caller: outside
    label: back up and note W
    action: "`POST /v1/backups` copies both stores with `VACUUM INTO` into a dated folder, and the operator notes the watermark W the record has reached."
    resource: internal/backup/backup.go#Take
    anchor: "func Take(ctx context.Context, d Deps) (api.Backup, error) {"
  - actor: /components/cli.md
    caller: outside
    label: stop, move the files aside, init
    action: "The operator stops the daemon and moves `ledger.db` and `tracking.db` aside, never deleting them: they are the record of what Anam knew until now, and receipts published from it verify only against that file. `anam init` at the same paths creates a ledger pinned to the same person and an empty operational store."
    resource: internal/app/init.go#initCmd
    anchor: 'Short: "idempotently create the ledger file and the operational store",'
  - actor: /components/daemon.md
    label: start with the bundled Definitions
    action: "`Open` runs `guardCatalog` over the new store, which holds nothing to disagree with, loads the catalog's Definitions and projection templates, then `persistCatalog` stores the Definitions and their history at once. An importer's registered Definition is not there yet."
    resource: internal/daemon/importers.go#persistCatalog
    anchor: "func persistCatalog(ctx context.Context, db *ops.DB, cat *catalog.Catalog) error {"
  - actor: /components/anam-submit.md
    caller: outside
    label: run each importer's poll
    action: "The operator runs every importer's `poll` through the gateway (`POST /plugin/<instance>/poll`). Each resends its source whole as the parts of one snapshot, and a registered type's Definition comes back with its first batch because the GET for that version is a 404."
    reads: [/data-structures/definition-manifest.md]
    writes: [/data-structures/batch-submission.md]
    resource: docs/IMPORTERS.md#8.8 Rebuilding the record from its sources
    anchor: "4. Run each importer's `poll` through the gateway"
  - actor: /components/daemon.md
    caller: /components/anam-submit.md
    label: admit every part, record the run
    action: "Each batch is admitted as in [admit a batch](/workflows/admit-a-batch.md), every facet as new because the ledger is empty, and the part's facet ids are recorded against the snapshot."
    writes: [/data-structures/snapshot-part.md]
    resource: internal/daemon/snapshots.go#recordSnapshotPart
    anchor: "rec, err := db.RecordSnapshotPart(ctx, ops.SnapshotPart{"
  - actor: /components/daemon.md
    caller: /components/anam-submit.md
    label: report the snapshot complete
    action: "When the last part lands, `GET /v1/snapshots/{id}` says `complete` and counts what the source's live facets of its types left absent. The operator checks each importer's last submit reports `snapshot complete, 0 of N live absent`."
    reads: [/data-structures/snapshot-part.md]
    resource: internal/daemon/snapshots.go#getSnapshot
    anchor: "rep.Live, rep.Absent = &live, &n"
  - actor: /components/mint.md
    caller: outside
    label: mint, then compare counts
    action: "`POST /v1/checkpoints` mints the new ledger's checkpoints, and the operator compares counts by type with the drill's report. A fact an operator had retracted while its source still holds it comes back live, so it is fixed at the source first."
    ends: true
    resource: internal/mint/mint.go#Report
    anchor: "if q.Full {"
---

Anam owns no facts, so a ledger can start again from nothing (card 141). The drill (`scripts/rebuild-drill.py` in anam-importers) proves it without touching the record: it runs each importer's `poll` and `anam_submit` into a scratch daemon on a new ledger and compares the result with the live one, type by type. The first run, on 2026-10-14, matched 2,939 facets in 18 types by facet id, content hash and status.

**What a rebuild cannot bring back.** What only the ledger held. Every fact is admitted at the time of the rebuild, so an as-of read before it, and every receipt naming the old ledger's W, has nothing to read in the new file; the old file keeps them. A source resends what it holds now, so superseded versions are gone. A fact retracted because its source dropped it stays gone; a source retired with `--retired` sends nothing, so its facets do not return. Whether a rebuild should keep the old anam-times and superseded versions (card 140's migration importer would) is open in IMPORTERS §6. The live rebuild itself, as opposed to the drill, has not been run (see [the drill is the only rebuild so far](/gaps/the-drill-is-the-only-rebuild-so-far.md)).

Related: [resend a source whole](/workflows/resend-a-source-whole.md), [any state is a checkpoint plus replay](/invariants/any-state-is-checkpoint-plus-replay.md), [the ledger is its own file](/decisions/the-ledger-is-its-own-file.md).
