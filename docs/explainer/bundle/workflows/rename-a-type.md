---
type: workflow
title: Rename a type
description: anam rename --table renames.json --yes carries every live facet of a type into another type as the next version of itself, same id, through the gate, so anam-time is kept and a read as-of before the run is unchanged.
timestamp: 2026-10-20T00:00:00Z
resource: internal/app/rename.go#planRename
anchor: "res, err := g.Rehearse(fctx, renameOf(f, to[f.Type]))"
basis: code
tier: supporting
trigger: The taxonomy renames a type (card 147). The operator stops the daemon, writes a table of types left and types arrived at, runs `anam rename --table renames.json` and reads the report, then runs it again with `--yes`.
steps:
  - actor: /components/cli.md
    caller: outside
    label: renameCmd reads its flags
    action: "`--table`, a JSON file of `{from, to}` pairs, and `--yes`. Without `--yes` the command only reports."
    resource: internal/app/rename.go#renameCmd
    anchor: 'cmd.Flags().BoolVar(&yes, "yes", false, "write the new versions; without it the command only reports")'
  - actor: /components/cli.md
    label: refuse to run against a daemon
    action: The run opens the files itself, as a retraction does; with `server` set it refuses and says to stop the daemon.
    resource: internal/app/rename.go#renameCmd
    anchor: 'if cfg.Server != "" {'
  - actor: /components/cli.md
    label: load and check the table
    action: Two valid types in every pair, none that goes nowhere, no type left twice, and no chain, so one run is one step and a rerun is all no-op. Unknown keys are refused.
    resource: internal/app/renametable.go#loadRenameTable
    anchor: "is both left and arrived at; one run is one step"
  - actor: /components/def.md
    caller: /components/cli.md
    label: the two types must key alike
    action: "`KeyingOf` reads the newest held body of the type left, as Guard does, and `SameKeying` compares it with the Definition in force for the type arrived at. A facet's id is the source and the identity values, so different keying would give different ids: duplicates beside the originals, not versions. The whole run is refused before the fold is read."
    reads: [/data-structures/definition-manifest.md]
    resource: internal/app/renamerun.go#checkRenameKeying
    anchor: "the ids would not meet"
  - actor: /components/read.md
    caller: /components/cli.md
    label: every live facet of a type left
    action: The canonical is composed from the last mint and its tail; the entries whose type the table leaves are kept, and their current versions fetched. A tombstoned facet is not in the fold, so it is not carried and stays as it was.
    reads: [/data-structures/fold-entry.md]
    resource: internal/app/rename.go#liveFacetsLeaving
    anchor: "return facetsOf(rctx, src.Ledger, entries)"
  - actor: /components/gate.md
    caller: /components/cli.md
    label: rehearse each facet
    action: "`Rehearse` runs every pillar a commit would, against the new type's Definition, with the facet's source, binding and payload as stored, then decides the disposition against the ledger as it stands, and writes nothing. The prepared envelope carries the type the facet is leaving, which nothing on the wire can set."
    resource: internal/gate/rename.go#Rehearse
    anchor: "p.renamedFrom = r.From"
  - actor: /components/gate.md
    label: supersede on a type change, only when marked
    action: "`disposition` refuses a commit whose facet is held under another type (card 105). A prepared envelope marked with the type it leaves, when the facet's current type is that one, supersedes instead, whether or not the payload changed: the hash is over the payload alone, so an unchanged payload would otherwise read as a no-op. The API path carries no mark."
    resource: internal/gate/commit.go#disposition
    anchor: 'if p.renamedFrom != "" && current.Type == p.renamedFrom {'
  - actor: /components/cli.md
    label: the report
    action: Per pair, how many facets the gate would carry and how many it would refuse, then a line per refusal naming the facet and the reason. A refusal anywhere ends the run with nothing written, even with `--yes`.
    resource: internal/app/renamerun.go#writeRenameReport
    anchor: 'FROM\tTO\tCARRY\tREFUSED'
  - actor: /components/cli.md
    branch: without --yes
    label: dry run, nothing written
    action: The report is printed and the command says to re-run with `--yes`.
    ends: true
    resource: internal/app/rename.go#renameCmd
    anchor: "dry run: nothing written. Re-run with --yes to carry them."
  - actor: /components/gate.md
    caller: /components/cli.md
    branch: with --yes
    label: carry them, in blocks
    action: "`RenameBatch` prepares a block of at most `MaxBatch` renames, marks each with the type it leaves, and writes them as `CommitBatch` does: the head read once, one transaction, retried on a stale watermark. Each facet gets its next version under the new type, the new Definition's classification and series, its own source, and the run as injector; the old version keeps its seq."
    resource: internal/gate/rename.go#RenameBatch
    anchor: "ps[i].renamedFrom = renames[i].From"
  - actor: /components/cli.md
    label: a block that fails names how far the run got
    action: The error says how many were carried of how many, and to re-run. A facet already carried sits under its new type, so the fold does not list it again: the run again plans only what is left.
    resource: internal/app/renamerun.go#renameAll
    anchor: "re-run to finish"
  - actor: /components/read.md
    label: what a reader sees
    action: A read as-of a W before the run is unchanged, old types and old projections; a read at the newest W shows the new types. Receipts cut before the run still verify. The operational store catches up by replay.
    ends: true
    resource: internal/gate/rename.go#Rename
    anchor: "Anam-time is kept"
---

Card 140. The alternative, a new ledger built facet by facet, flattened anam-time to the migration moment and split receipts from their ledger; this keeps one ledger and adds one version per facet. The type is not in a facet's id, so the new version lands on the same facet; `type` is a column on every version row, so nothing is overwritten and the append-only triggers permit the insert. The one concession the gate makes is scoped by the call, and `ErrTypeChanged` guards every other path as before.
