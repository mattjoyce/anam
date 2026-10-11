---
type: workflow
title: Retract from a snapshot
description: anam retract bp-csv --yes plans what the importer's newest complete resend did not carry, guards the plan, and tombstones each facet through the gate as one anam-event.
timestamp: 2026-10-11T00:00:00Z
resource: internal/app/retract.go#planFromSnapshot
anchor: "snap, ok, err := db.NewestCompleteSnapshot(ctx, source)"
basis: code
tier: supporting
trigger: An importer's `poll` run completed a snapshot and reported facets absent; the operator stops the daemon (retract refuses to run with a server configured) and runs `anam retract bp-csv`, then again with `--yes`.
steps:
  - actor: /components/cli.md
    caller: outside
    label: retractCmd reads its flags
    action: "`--yes`, `--max-fraction` (a tenth by default), `--retired`, `--imports`."
    resource: internal/app/retract.go#retractCmd
    anchor: 'cmd.Flags().Float64Var(&maxFraction, "max-fraction", 0.1,'
  - actor: /components/cli.md
    label: refuse to run against a daemon
    action: "`loadConfig` opens local files and refuses when `server` is set: the daemon owns them. There is no retract route or client method."
    reads: [/data-structures/config-keys.md]
    resource: internal/app/app.go#loadConfig
    anchor: "opens local files, and with server %s configured the daemon owns them"
  - actor: /components/cli.md
    label: declared import or importer source?
    action: A source named in the imports file is re-scanned and compared with the live set; a source no import declares is an importer's, planned from its snapshot. `--retired` instead plans every live facet of the source and refuses one still declared anywhere.
    resource: internal/app/retract.go#retractCmd
    anchor: "// No import declares it, so it is an importer's source"
  - actor: /components/store-ops.md
    caller: /components/cli.md
    label: find the newest complete snapshot
    action: "`NewestCompleteSnapshot` reads the latest `snapshot` row for the source with a `completed_at`, and `SnapshotFacets` the ids its parts carried, no-ops included."
    reads: [/data-structures/snapshot-part.md]
    resource: internal/store/ops/snapshot.go#NewestCompleteSnapshot
    anchor: "SELECT id FROM snapshot WHERE source = ? AND completed_at IS NOT NULL"
  - actor: /components/cli.md
    branch: no complete snapshot
    label: refuse, nothing to plan from
    action: The error names the source and says it has sent no complete snapshot to plan from.
    ends: true
    resource: internal/app/retract.go#planFromSnapshot
    anchor: "no import declares source %q, and it has sent no complete snapshot to plan from"
  - actor: /components/read.md
    caller: /components/cli.md
    branch: a complete snapshot exists
    label: absent = live, not carried, untouched
    action: "`Absent` reads the canonical composed from the last mint run and its tail, keeps the source's live facets of the snapshot's types that the run did not carry, and drops any touched since `started_w`: a facet an incremental run sent beside the resend is never called absent."
    reads: [/data-structures/snapshot-part.md, /data-structures/fold-entry.md]
    resource: internal/read/snapshot.go#Absent
    anchor: "continue // touched since the run began: not its to judge"
  - actor: /components/cli.md
    label: guard the plan
    action: A source that holds nothing is refused outright, and a plan over `--max-fraction` of the source's live facets is refused unless the operator raises it. The report prints the snapshot id, when it began and completed, and each facet to go.
    resource: internal/app/retract.go#guardRetraction
    anchor: "if fraction := float64(absent) / float64(total); fraction > maxFraction {"
  - actor: /components/cli.md
    branch: without --yes
    label: dry run, nothing written
    action: The plan is printed and the command says to re-run with `--yes`.
    ends: true
    resource: internal/app/retract.go#retractCmd
    anchor: "dry run: nothing written. Re-run with --yes to tombstone them."
  - actor: /components/cli.md
    branch: with --yes
    label: open the ledger for writing, one gate
    action: "`retractAll` opens the ledger read-write, builds a gate with the built-in scopes, and calls `Retract` per facet under the configured timeout; a failure part-way says how many landed and to re-run, since a tombstoned facet drops out of the next plan."
    resource: internal/app/retract.go#retractAll
    anchor: "res, _, err := g.Retract(ctx, f.ID)"
  - actor: /components/gate.md
    caller: /components/cli.md
    label: a tombstoned event, no artifact
    action: "`Retract` folds the facet's own events, returns no-op if already tombstoned, and otherwise builds a `tombstoned` event against the current artifact version at the next seq, retrying a stale watermark up to five times."
    resource: internal/gate/commit.go#Retract
    anchor: "Kind: facet.EventTombstoned, FacetID: facetID, AnamTime: g.clock.Now(),"
  - actor: /components/store-ledger.md
    caller: /components/gate.md
    label: Mark writes the event in one transaction
    action: "`Mark` is the ledger's second primitive: an event with no artifact, under the same stale-watermark check as `AdmitBatch`. W advances by one per facet."
    writes: [/data-structures/anam-event.md]
    resource: internal/store/ledger/write.go#Mark
    anchor: "writes an anam-event in one transaction"
  - actor: /components/cli.md
    label: report; the index catches up later
    action: Prints how many were tombstoned and that the ledger moved. `retract` does not run catch-up, so the operational index is behind until the daemon starts and takes its first write.
    ends: true
    resource: internal/app/retract.go#retractAll
    anchor: 'tombstoned %d facets; the ledger moved\n'
---

Retraction is a command, never a side effect of ingest: a row missing from one scan and a source gone look alike from inside an envelope, so no ingest tombstones. The guards exist because a tombstone cannot be undone by a resend; a resend of the same row is a no-op.

The retired variant (card 120) is the cut-over tool: once the new importer's complete snapshot reports 0 absent and the old source's declaration is removed, `anam retract health-db --retired --yes --max-fraction 1` tombstones every live facet of the old source, and a read as of before still shows them.

Related: [retraction by complete resend](/decisions/retraction-by-complete-resend.md), [retract runs locally, not over the daemon](/gaps/retract-runs-locally-only.md).
