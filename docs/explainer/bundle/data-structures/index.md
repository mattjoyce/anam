# Data structures

* [Admission response](admission-response.md) - What POST /v1/envelopes answers: w, changed, one positional result per envelope, and snapshot progress; or a 422 naming every refused envelope.
* [Anam-event row](anam-event.md) - One row per change in Anam's knowledge: a gapless seq, one of five kinds, the facet and artifact version it concerns, and anam-time.
* [Batch event](batch-event.md) - The ductile event <plugin>.batch an importer emits and an anam_submit instance consumes: envelopes, the manifests as text, a label, and optionally a snapshot part or current_versions.
* [Batch submission](batch-submission.md) - The JSON body of POST /v1/envelopes: 1 to 49 envelopes and an optional snapshot label; any unknown field is a 400.
* [Checkpoint log](checkpoint-row.md) - The operational store's checkpoint, mint_run, mint_seal and checkpoint_size tables: one row per mint of a bucket, the runs that checked every bucket, and the seal that proves no crashed run left rows behind.
* [Checkpoint value file](checkpoint-value.md) - The live-only value of one bucket at a watermark: a JSON array of fold entries, written under its own sha256 in the artifact directory.
* [Config keys](config-keys.md) - Every key the daemon and CLI read, from config.yaml or ANAM_* env vars, with its default; an unknown key or an unknown bucket width is refused at load.
* [Definition manifest](definition-manifest.md) - The JSON contract for one facet type: meta with identity, time, classification, validation mode and series, and a closed JSON Schema 2020-12.
* [Envelope](envelope.md) - The one message that enters the gate; the Go struct the daemon builds from a submitted envelope, with source and injector set from the token.
* [Facet artifact row](facet-artifact.md) - One row per artifact version in the ledger's facet table: the governing metadata as columns, the payload as an opaque BLOB, and no status column.
* [Facet index row](facet-index-row.md) - The operational store's derived index of each facet's current version, status, fact-time and bucket, written by catch-up and provably rebuildable by replaying the ledger.
* [Fold entry](fold-entry.md) - One live facet in a canonical value: its governing metadata, derived status and payload, with no artifact version and no watermark so the hash is stable across rebuilds.
* [Importers file](importers-file.md) - The YAML the daemon reads once at start naming each importer's source, the SHA-256 of its token, and the types it may submit and register.
* [Projection template](projection-template.md) - An embedded JSON manifest naming a versioned prune: types, statuses, a classification ceiling, a fact-time window and a shape.
* [Receipt](receipt.md) - Proof of what a projection served: the served hash, the facet ids, the watermark and ledger id, the template and definition-set versions, served_at, the span, and the query a read narrowed by.
* [Snapshot tables](snapshot-part.md) - The operational store's record of an importer's resend-everything run: the run with its started and completed watermarks, each part's digest, and the facet ids the parts carried.
* [Timeline response](timeline-response.md) - What GET /v1/timeline answers: the watermark, counts, the whole-person hash, the shown entries with their versions, and verdicts when a projection or explain was asked for.
