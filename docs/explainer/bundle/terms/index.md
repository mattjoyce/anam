# Terms

* [Anam-event](anam-event.md) - An assertion that Anam's knowledge changed, in one of exactly five kinds, each with a gapless sequence number.
* [Anam-time](anam-time.md) - When Anam's knowledge changed; the clock an as-of reads, and an audit instrument rather than a history one.
* [Bucket span](bucket-span.md) - The minted span, a calendar window over fact-time that every facet belongs to by arithmetic; invisible to the reader.
* [Attention (the mind domain)](attention.md) - What the person says is absorbing them, kept as a reading with a topic and a strength from 1 to 5, and as a declared start and end; the first concept in the mind domain.
* [Canonical](canonical.md) - The live-only value of the fold at a watermark; superseded and tombstoned facets are excluded from it.
* [Checkpoint](checkpoint.md) - An immutable, content-addressed value of the live-only fold over one bucket at a watermark, there only to bound replay.
* [Derived span](derived-span.md) - A lens the person names over the timeline, composed on demand from bucket values and never minted.
* [Employment (the org domain)](employment.md) - Where the person worked and in what role, kept as a start, changes of role and an end, one series per period with an employer; the first concept in the org domain.
* [Envelope](envelope.md) - The one message an importer hands the gate; the only way anything enters the ledger.
* [Error kernel](error-kernel.md) - The small trusted core that must not corrupt: the append-only ledger plus the gate's admission invariants.
* [Facet](facet.md) - The atomic unit Anam keeps, a source fragment held verbatim plus the metadata that governs it.
* [Facet Definition](facet-definition.md) - The versioned contract for one facet type, from its closed JSON Schema to how identity and fact-time are read from a payload.
* [Fact-time](fact-time.md) - When a fact was true in the world; the axis the timeline is ordered by, promoted out of the payload at the gate.
* [Fold](fold.md) - The computation that derives each facet's current status from its anam-events and keeps the live ones.
* [Idempotency](idempotency.md) - Re-ingesting the same source row is a no-op; what makes ingest crash-clean and a resend always safe.
* [Importer](importer.md) - A ductile plugin outside Anam that reads one source, shapes rows to a Definition, and submits them under its own token.
* [Intake (caffeine, alcohol and notable meals)](intake.md) - What the person says they took in, kept as a fact at its minute so a later question can ask whether it came before a poor sleep or a reading; drinks with a count, meals in the person's words, and no nutrition.
* [Ledger](ledger.md) - Anam's append-only record of everything it admitted, every facet kept and marked at its current status.
* [Mint](mint.md) - Checkpointing per bucket after the importer batches that moved W, rebuilding only the buckets the tail touched.
* [Operational store](operational-store.md) - The second SQLite file, authoritative for what Anam authors, with each table annotated by where it comes back from.
* [Principal](principal.md) - A caller the daemon admits, with a name, a role and the SHA-256 of its token; an importer principal also has the one source it submits as and the types it may submit.
* [Projection](projection.md) - A versioned template that prunes the canonical by metadata only; a consumer selects one and never defines one.
* [Purchase (the money domain)](purchase.md) - One order or receipt from a merchant, kept as a single fact with its lines and totals as the document states them; the first concept in the money domain.
* [Query](query.md) - The third read after the timeline and fields; a Datalog program evaluated over the fold at a watermark, through a projection, answering with a derived value and a receipt. Decided 2026-10-17 and built 2026-10-19 (card 173), reached by `anam query`, `POST /v1/query` and `GET /v1/query`.
* [Receipt](receipt.md) - Proof of exactly what a projection served at a watermark, verifiable from its stored row alone or against the ledger.
* [Role](role.md) - What a principal's token opens, one of reader, importer or operator; each route names the roles it admits and no handler checks who is calling.
* [Series](series.md) - Which facets are repeated statements about one thing, stamped at the gate so a snapshot can keep the latest per thing.
* [Snapshot (read)](snapshot-read.md) - The last-known-value read: the latest facet per series at or before a fact-time, closed series dropped.
* [Snapshot (resend)](snapshot-resend.md) - An importer's labelled resend-everything run, from which Anam reports what the source no longer holds.
* [Source](source.md) - A capture origin, named lower-case hyphenated; part of every facet's identity and provenance.
* [Source ref](source-ref.md) - The deterministic natural key saying which source row a facet is, composed from identity fields and never from values.
* [Status](status.md) - A facet's current state, derived by the fold from anam-events and never stored; distinct from its immutable birth status.
* [Supersession](supersession.md) - What happens when a source row changes: a new artifact version and a superseded event against the prior, nothing mutated.
* [The whole-person read](inversion.md) - The person is the substrate and the whole-life read is the default.
* [Tombstone](tombstone.md) - A tombstoned anam-event marking a facet retracted without deleting it; never a new artifact.
* [Watermark](watermark.md) - W, the seq of the newest anam-event; the position every checkpoint and receipt is pinned to.
