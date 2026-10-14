# Workflows

* [Admit a batch](admit-a-batch.md) - One blood-pressure export dropped in a watched folder becomes a facet in the ledger, an index row, and a minted checkpoint, with every call between the importer repo and the service traced.
* [Hand Anam a document](hand-anam-a-document.md) - anam_documents names a scanned letter and its sidecar in a batch; anam_submit posts the bytes to POST /v1/documents, the daemon stores them in the library through the write queue, and the doc.reference that names them is admitted only if the library holds those exact bytes.
* [Mint checkpoints](mint-checkpoints.md) - anam checkpoint over the daemon rebuilds only the buckets the events since the last run touched, writes a new version for each whose hash moved, and seals the log.
* [Publish a projection](publish-a-projection.md) - anam serve bp prunes the canonical at the head with the bp template, writes the served value and a receipt that pins what was shown, and prints the receipt hash.
* [Read the timeline](read-the-timeline.md) - anam timeline --from 2026-01-01 --to 2026-02-01 against the daemon, answered by composing the last mint's checkpoints with the tail of events, or by a full fold when the checkpoint log cannot be trusted.
* [Register a Definition](register-a-definition.md) - anam_submit finds Anam does not publish a Definition version the batch carries and registers it; the daemon checks it against the bundled set, the held versions and the keying rule before holding it immutably.
* [Resend a source whole](resend-a-source-whole.md) - anam_records sends its folder as one snapshot of the types it carries; the daemon binds each part to its envelopes, completes the run on the last part, and reports what is live but was not carried.
* [Retract from a snapshot](retract-a-source.md) - anam retract bp-csv --yes plans what the importer's newest complete resend did not carry, guards the plan, and tombstones each facet through the gate as one anam-event.
