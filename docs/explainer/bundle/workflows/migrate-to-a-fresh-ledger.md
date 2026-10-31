---
type: workflow
title: Migrate to a fresh ledger
description: anam migrate --from old --to new --yes carries the live facets of one ledger into a new ledger through the gate, each under the type's current Definition version when it comes out the same, otherwise under the one it was admitted under, so ids, hashes and fact times are the old ones and anam-time restarts at the cut.
timestamp: 2026-10-22T00:00:00Z
resource: internal/app/migrate.go#migrateCmd
anchor: "func migrateCmd(cfgPath *string) *cobra.Command {"
basis: code
tier: supporting
trigger: The record has gathered churn, superseded versions and retired types, and the operator wants a ledger holding what is true now with the old one kept as the archive (card 200). The daemon is stopped, the importers are paused, the two files are backed up, and the operator runs `anam migrate` to new file names, first without `--yes` to read the report.
steps:
  - actor: /components/cli.md
    caller: outside
    label: migrateCmd reads its flags
    action: "`--from` and `--from-store` name the old pair, `--to` and `--to-store` the new one, `--program` a file defining `pick/1`, and `--yes`. Without `--yes` the command only reports. It reads the person and the time bounds from the config but touches no daemon."
    resource: internal/app/migrate.go#migrateCmd
    anchor: 'f.StringVar(&o.fromStore, "from-store", "", "the old operational store file, opened read-only")'
  - actor: /components/cli.md
    label: refuse a target that exists
    action: "The new ledger and store must not exist, nor their `-wal`, `-shm` or `-journal` files, and must not be the same path. The check runs before anything is read, on a dry run too."
    resource: internal/migrate/carry.go#CheckAbsent
    anchor: 'return fmt.Errorf("%w: %s", ErrExists, p)'
  - actor: /components/store-ops.md
    caller: /components/cli.md
    label: read the registered rows
    action: "`ReadRegistered` reads, in one read transaction on a handle SQLite will not write through, every `registered_definition` and `registered_projection` row with its registration moment, every template body and admission hold, every receipt, served value, saved selection and selection code (card 224), and the store's copy of the Definitions the binary carried. The old files are opened read-only throughout."
    resource: internal/store/ops/registered.go#ReadRegistered
    anchor: "ReadOnly: true"
  - actor: /components/fold.md
    caller: /components/cli.md
    label: fold the old ledger to its head
    action: "The one fold, `fold.Read`, replays every anam-event and fetches each live facet at the version in force. Tombstoned facets and superseded versions are not in the result, so they are never carried."
    reads: [/data-structures/anam-event.md]
    resource: internal/migrate/plan.go#Read
    anchor: "res, err := fold.Read(rctx, l)"
  - actor: /components/read.md
    caller: /components/cli.md
    label: select, by program or all
    action: "With no program, every live facet. With `--program`, `read.Evaluate` runs the program over the canonical, the whole live value with no projection narrowing, and the facets whose ids `pick/1` holds are kept. An id picked that no live facet holds is reported and not carried."
    resource: internal/migrate/plan.go#selectFacets
    anchor: "read.Evaluate(ctx, res.Value, res.Live, current, program, selection.Relation, 0)"
  - actor: /components/def.md
    caller: /components/cli.md
    label: hold every Definition version
    action: "`NewVersions` holds the Definition in force, then every body of the registered rows, the binary's manifests and history, and the store's provisioned copy that still parses as a contract. A body that does not parse, like `health.weight` v1 with its retired `lenient` mode, is not a contract and is reported with its parse error."
    resource: internal/def/versions.go#NewVersions
    anchor: "A body that no longer parses is not a contract."
  - actor: /components/gate.md
    caller: /components/cli.md
    label: vet each facet
    action: "`VetCarry` runs every pillar that needs no ledger read against one Definition version: scope, Definition, assurance floor, size, schema, identity, and the facet it would build. The plan asks it of the type's current version first and accepts that if the facet built has the same id, content hash, fact time, series, classification, descriptor and binding as the one read, so only the Definition version differs (a move, flagged in the report). If not, it asks again of the version the facet was admitted under and holds it to the same. A facet neither carries is refused. Into an empty ledger these pillars are all that can refuse."
    resource: internal/gate/carry.go#VetCarry
    anchor: "return g.build(p, 1)"
  - actor: /components/cli.md
    label: the report
    action: "The old head and how many facets are live, then per type and source how many would be carried, how many of those move to the current Definition version and how many keep their own, and how many refused, a line for each group moved or kept, one line for each group of refusals with a reason, and a line for each `record.edge` end the selection does not carry, with what the old ledger holds of it. Any refusal ends the run with nothing written, even with `--yes`."
    resource: internal/app/migrate.go#writeMigratePlan
    anchor: '"TYPE\tSOURCE\tCARRY\tMOVED\tOWN\tREFUSED"'
  - actor: /components/cli.md
    branch: without --yes
    label: dry run, nothing written
    action: The report is printed and the command says to re-run with `--yes`. No file was made.
    ends: true
    resource: internal/app/migrate.go#run
    anchor: 'dry run: nothing written. Re-run with --yes to carry %d facets.'
  - actor: /components/store-ledger.md
    caller: /components/cli.md
    branch: with --yes
    label: create the new pair
    action: "`ledger.Create`, the code `anam init` runs, makes the new ledger with a fresh ledger id; `ops.Open` makes the new operational store. A run that fails after this removes the files it made, so a rerun finds none."
    resource: internal/migrate/carry.go#createTarget
    anchor: "ledger.Create(ctx, to.Ledger, p.Person)"
  - actor: /components/store-ops.md
    caller: /components/cli.md
    label: Definitions first
    action: "`CarryRegistered` writes every registered Definition and projection, at its registration moment, and the rest of the authored tier, each content row checked against its key, in one transaction before any facet, so every version a carried facet was admitted under is held in the new store. The bundled ones come from the binary when a daemon starts on it."
    resource: internal/store/ops/registered.go#CarryRegistered
    anchor: "ops: carry registered"
  - actor: /components/gate.md
    caller: /components/cli.md
    label: carry them, in blocks
    action: "`CarryBatch` prepares a block of at most `MaxBatch` facets against the versions the plan chose for them, then writes it as `CommitBatch` does: the head read once, one transaction, retried on a stale watermark. Each facet is a first admission with its source, injector, source ref, binding and payload as stored, so its id, hash and fact time come out as they were; the block shares one anam-time."
    resource: internal/gate/carry.go#CarryBatch
    anchor: "g.prepareCarry(carries[i])"
  - actor: /components/fold.md
    caller: /components/cli.md
    label: prove the new ledger is the selection
    action: "The new ledger is opened read-only and folded. Its live facets must be the selection's, facet for facet by id, content hash, fact time and status, with none extra and none missing. The report prints both heads and both sets of file sizes."
    resource: internal/migrate/carry.go#Prove
    anchor: "was selected and is not live in the new ledger"
  - actor: /components/cli.md
    label: a second dry run is the same set
    action: "The plan is read again with the new pair as `--from`, and must select the same ids with the same content hashes, types and sources, all of them carriable. That is the carry's idempotence: migrating the new ledger would change nothing."
    ends: true
    resource: internal/migrate/plan.go#Same
    anchor: "is selected now and was not"
---

Card 200. The alternative, re-importing every source into a fresh ledger (the [rebuild](rebuild-the-record.md)), needs each importer's source and resends what the ledger already holds; this reads the ledger. A facet's id is its source and identity values and its content hash is over its payload alone, so carrying it unchanged through the gate gives the id and hash it had. The fact time is read from the payload by a Definition version, so the carry holds a facet to the current version only when the facet built comes out identical, and otherwise to the version it was admitted under.

Anam-time restarts: every facet is admitted at the cut, one event each, so the new ledger's W is the number of facets carried and no as-of read before the cut can be answered from it. The old pair, kept read-only, answers those, and every old receipt. The operational store catches up by replay when a daemon starts on the new ledger.

The gate's concession is scoped by the call, as the rename's is. `CarryBatch` and `VetCarry` resolve the Definition from a `Held` the gate was given with `Holding`; a gate built without one refuses a carry, and nothing on the wire sets a version, so the API's refusal of an envelope shaped to another version stands.

A facet admitted under a version that does not parse as a contract today cannot be held to it, so the plan tries the current version, and refuses the facet only if that fails too. The 1,600 live `health.weight` facets from `records` are admitted under v1, whose words use the retired `lenient` mode (card 52). Their payloads pass the current version with the same id, hash and fact time, so they move to it, flagged "moved from v1" (Matt, 2026-10-22). The proof after `--yes` holds identity to id, content hash, fact time and status, and reports the moves apart.
