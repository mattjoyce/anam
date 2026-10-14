# Terms

* [Anam-event](anam-event.md) - An assertion that Anam's knowledge changed, in one of exactly five kinds, each with a gapless sequence number.
* [Anam-time](anam-time.md) - When Anam's knowledge changed; the clock an as-of reads, and an audit instrument rather than a history one.
* [Bucket span](bucket-span.md) - The minted span, a calendar window over fact-time that every facet belongs to by arithmetic; invisible to the reader.
* [Canonical](canonical.md) - The live-only value of the fold at a watermark; superseded and tombstoned facets are excluded from it.
* [Checkpoint](checkpoint.md) - An immutable, content-addressed value of the live-only fold over one bucket at a watermark, there only to bound replay.
* [Derived span](derived-span.md) - A lens the person names over the timeline, composed on demand from bucket values and never minted.
* [Envelope](envelope.md) - The one message an importer hands the gate; the only way anything enters the ledger.
* [Error kernel](error-kernel.md) - The small trusted core that must not corrupt: the append-only ledger plus the gate's admission invariants.
* [Facet](facet.md) - The atomic unit Anam keeps, a source fragment held verbatim plus the metadata that governs it.
* [Facet Definition](facet-definition.md) - The versioned contract for one facet type, from its closed JSON Schema to how identity and fact-time are read from a payload.
* [Fact-time](fact-time.md) - When a fact was true in the world; the axis the timeline is ordered by, promoted out of the payload at the gate.
* [Fold](fold.md) - The computation that derives each facet's current status from its anam-events and keeps the live ones.
* [Idempotency](idempotency.md) - Re-ingesting the same source row is a no-op; what makes ingest crash-clean and a resend always safe.
* [Importer](importer.md) - A ductile plugin outside Anam that reads one source, shapes rows to a Definition, and submits them under its own token.
* [Ledger](ledger.md) - Anam's append-only record of everything it admitted, every facet kept and marked at its current status.
* [Mint](mint.md) - Checkpointing per bucket after the importer batches that moved W, rebuilding only the buckets the tail touched.
* [Operational store](operational-store.md) - The second SQLite file, authoritative for what Anam authors, with each table annotated by where it comes back from.
* [Projection](projection.md) - A versioned template that prunes the canonical by metadata only; a consumer selects one and never defines one.
* [Receipt](receipt.md) - Proof of exactly what a projection served at a watermark, verifiable from the artifact alone or against the ledger.
* [Series](series.md) - Which facets are repeated statements about one thing, stamped at the gate so a snapshot can keep the latest per thing.
* [Snapshot (read)](snapshot-read.md) - The last-known-value read: the latest facet per series at or before a fact-time, closed series dropped.
* [Snapshot (resend)](snapshot-resend.md) - An importer's labelled resend-everything run, from which Anam reports what the source no longer holds.
* [Source](source.md) - A capture origin, named lower-case hyphenated; part of every facet's identity and provenance.
* [Source ref](source-ref.md) - The deterministic natural key saying which source row a facet is, composed from identity fields and never from values.
* [Status](status.md) - A facet's current state, derived by the fold from anam-events and never stored; distinct from its immutable birth status.
* [Supersession](supersession.md) - What happens when a source row changes: a new artifact version and a superseded event against the prior, nothing mutated.
* [The inversion](inversion.md) - The one decision that distinguishes Anam from Arch: the person is the substrate and the whole-life read is the default.
* [Tombstone](tombstone.md) - A tombstoned anam-event marking a facet retracted without deleting it; never a new artifact.
* [Watermark](watermark.md) - W, the seq of the newest anam-event; the position every checkpoint and receipt is pinned to.
