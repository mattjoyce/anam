# The series key: design pass, 2026-10-07

Hickey × Armstrong, Before pass, on kanban card 15 before any code. The card's acceptance
criteria are the spec; this note records what the lens changed and what it confirmed.

## The shape

A Definition may declare one `series` block:

```json
"series": {"name": "med.regimen", "key": ["/drug", "/course"], "closes": false}
```

- `name` is a dotted, namespaced label like a facet type. It is the family the series belongs
  to, shared by every type that participates (`med.started`, `med.dose-changed`, `med.stopped`
  all say `med.regimen`).
- `key` is the ordered pointers whose values name one series within the family. The gate
  composes the stamp as `<name>:<v1>[:<v2>...]`, the same shape as `source_ref`.
- `closes` says that a facet of this type ends the series it belongs to.

The stamp is written onto the facet as metadata at admission, beside classification and
fact-time, and the ledger holds it in a column. The fold carries it on each entry. The snapshot
read groups on it and never opens a payload.

## Hickey: what the lens changed

**Two declarations were one braid.** The card asked whether a series key is one declaration or
two (group and course). It is one key with two elements, but there was a third thing hiding:
the *family*. Without a name, `med.started` stamping `ramipril:1` and `illness.started`
stamping `flu:1` share nothing but could collide, and three manifests would have to agree on a
key with nothing to check them against. Naming the family makes the agreement checkable: the
registry refuses two Definitions that give one family two different keys, at load, before any
ingest.

**Stamp at capture, not at read.** The alternative was to resolve the series at fold time from
the current Definition. That braids a read with the contract in force *today*, so an artifact
admitted under definition v1 would regroup silently when v2 changed the key, which is exactly
what `definition_version` on the facet exists to prevent. Stamping makes the series part of the
admitted value, epochal like fact-time. The cost is honest: a later key change reaches only
facets admitted after it, because admission is idempotent on content. Noted, not solved.

**Closing stays a rule, not a stamp.** Whether a type closes is a property of the type, consulted
when the snapshot is taken. It is data in the Definition, read through the registry at read
time, and it is not copied onto the facet. A facet does not know it ended something.

**No series is the absence of the attribute, not an empty string.** The fold entry carries
`series` with `omitempty`, so every existing checkpoint serialises to the same bytes as before.
The migration is value-preserving, and `anam checkpoint --dry-run` minting nothing is the proof.

## Armstrong: what the lens confirmed

**The migration touches the error kernel, so it is small and whole.** One transaction: add the
column with a default, record `schema_migration:2` in `ledger_meta`. The pin table is write-once
by trigger, so the version history is itself append-only: a created-at version plus the
migrations applied. A crash mid-migration rolls the transaction back and the next `anam init`
runs it again. Nothing is dropped, no trigger is touched, and DDL does not fire row triggers.

**Readers fail fast until the writer migrates.** `Open` and `OpenReadOnly` compute the effective
version and refuse a ledger behind this build with the existing `ErrSchemaMismatch`, which
already says migrate, never delete. The hourly ingest fails loudly until `anam init` runs once.
An older binary sees its own pin and keeps working; it has no series types to write.

**Resolution failure rejects at the gate.** A series-bearing Definition whose pointers do not
resolve is non-compliant, like a missing identity. The batch fails whole, as it does today.

**The snapshot is pure.** A function from a value and a fact-time to a value, with the registry
supplying which types close. It has no state and nothing to restart.

## Card 23, same day

The snapshot became a projection **shape** (`"shape": "snapshot"`, defaulting to `log`), because a
consumer selects a projection and never defines one (journal: two-clocks). The relation is decided
once, in `fold.Fates`, and both `fold.Snapshot` and the projection's explain are built on it, so the
reason a facet is not served ("displaced by", "closed by") comes from the same procedure that served
the rest. The closing rule reaches the projection as a parameter at the point of use, like the consent
gate, and a snapshot template without it is refused rather than served wrong. `timeline --at` is the
person's own view of the same read; `serve meds` is the published one with a receipt.

## Deferred

- A series stamp in `facet_index` for the inspector (card 21).
- Re-stamping existing facets after a Definition changes its key.
- A fact-time `at` on the inspector's projection page; the page shows the shape's verdicts but
  only at the head of fact-time.
