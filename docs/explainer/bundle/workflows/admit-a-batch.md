---
type: workflow
title: Admit a batch
description: One blood-pressure export dropped in a watched folder becomes a facet in the ledger, an index row, and a minted checkpoint, with every call between the importer repo and the service traced.
timestamp: 2026-10-11T00:00:00Z
resource: internal/daemon/envelopes.go#postEnvelopes
anchor: "envs, snap, err := decodeSubmission(w, r, imp, d.cfg.Limits)"
basis: code
tier: core
trigger: The ductile gateway's folder_watch sees `BloodPressure_11-July-2026` under the export folder and emits `bp.export`; the pipeline runs `anam_bp_csv handle`. The example reading is `2026-1-24 06:30:00`, reading 1, 128/79 mmHg, from source `bp-csv`.
steps:
  - actor: /components/importer-bp-csv.md
    caller: outside
    label: run handle on the dropped export
    action: ductile spawns `bp_csv/run.py` with command `handle` and the event payload `{path, change_type}` on stdin; a deleted path returns no events.
    resource: ../anam-importers/bp_csv/run.py#handle
    anchor: 'if event.get("change_type") == "deleted":'
  - actor: /components/importer-bp-csv.md
    label: read the CSV as Go's encoding/csv would
    action: "Every row becomes `{slug: text}`, blanks dropped, and readings that share a timestamp get an ordinal in `reading`. An export whose record boundaries cannot be read for certain is refused whole, nothing of it sent."
    resource: ../anam-importers/bp_csv/run.py#payloads
    anchor: 'row["reading"] = str(ordinal[ts])'
  - actor: /components/importer-bp-csv.md
    label: shape each row to the OMH body
    action: "`omh` builds the Open mHealth blood-pressure-4.0 fields (systolic and diastolic with unit, effective_time_frame in the Sydney offset) and keeps every export column as text beside them. Posture `Seated` maps to `sitting`; an unmapped posture stays and the schema refuses it."
    resource: ../anam-importers/bp_csv/run.py#omh
    anchor: 'POSTURES = {"Seated": "sitting"}'
  - actor: /components/importer-base.md
    caller: /components/importer-bp-csv.md
    label: wrap the payload in an envelope
    action: "`envelope` yields `{type: health.blood-pressure, definition_version: 5, payload, asserted_person}` and nothing else: no source, no source_ref."
    writes: [/data-structures/envelope.md]
    resource: ../anam-importers/anam_importer/envelopes.py#envelope
    anchor: 'env["asserted_person"] = asserted_person'
  - actor: /components/importer-base.md
    caller: /components/importer-bp-csv.md
    label: validate locally with jsonschema
    action: "`check` runs every payload through the Definition's closed schema with Draft202012Validator and raises `Invalid` with every problem, so a refusal at the gate later means the importer and Anam disagree, not that a row was malformed."
    reads: [/data-structures/definition-manifest.md]
    resource: ../anam-importers/anam_importer/envelopes.py#check
    anchor: "raise Invalid(problems)"
  - actor: /components/importer-base.md
    caller: /components/importer-bp-csv.md
    label: cut into batches of at most 49
    action: "`batches` slices in order at `MAX_BATCH = 49`, the gate's batch, one transaction."
    resource: ../anam-importers/anam_importer/envelopes.py#batches
    anchor: "MAX_BATCH = 49"
  - actor: /components/importer-bp-csv.md
    label: emit one anam_bp_csv.batch per group
    action: Each group becomes a ductile event carrying the envelopes, the Definition manifest as text, and a `batch` label `<file> i/n`. A `poll` run would also label each as a snapshot part; `handle` does not.
    writes: [/data-structures/batch-event.md]
    resource: ../anam-importers/bp_csv/run.py#batch_events
    anchor: '"batch": f"{path.name} {i + 1}/{len(groups)}"'
  - actor: /components/anam-submit.md
    caller: outside
    label: the pipeline runs anam_submit handle
    action: ductile routes `anam_bp_csv.batch` to the `anam_submit_bp_csv` instance and runs its `handle` with the instance config (`anam_url`, `token_secret`) and the vault secret the config names.
    reads: [/data-structures/batch-event.md]
    resource: ../anam-importers/anam_submit/run.py#handle
    anchor: "batch = ductile.payload_of(request)"
  - actor: /components/importer-base.md
    caller: /components/anam-submit.md
    label: make sure Anam publishes the Definition
    action: "`ensure_published` GETs `/v1/definitions/health.blood-pressure/5`; a 404 would POST the manifest (see [register a Definition](/workflows/register-a-definition.md)); a different body at that version stops the run. The type is bundled, so this is a 200 and nothing is posted."
    reads: [/data-structures/definition-manifest.md]
    resource: ../anam-importers/anam_importer/definitions.py#ensure_published
    anchor: "published = client.definition(d.type, d.version)"
  - actor: /components/importer-base.md
    caller: /components/anam-submit.md
    label: POST /v1/envelopes with the bearer token
    action: "`Client.submit` posts `{\"envelopes\": [...]}` with `Authorization: Bearer <token>` and an `Anam-Timeout` header, over `http.client` so no redirect is followed, under one deadline for the whole call."
    writes: [/data-structures/batch-submission.md]
    resource: ../anam-importers/anam_importer/client.py#submit
    anchor: 'status, body = self._call("POST", "/v1/envelopes", payload)'
  - actor: /components/daemon.md
    caller: /components/importer-base.md
    label: resolve the token to a principal
    action: "`admit` hashes the presented token with SHA-256 (`who`) and looks it up in the map loaded from the importers file at start. `POST /v1/envelopes` admits the importer role only, so the operator's or a reader's token is a 403 naming its principal and role; an unknown token is a 401. The importer's source and types ride on the request context."
    reads: [/data-structures/importers-file.md]
    resource: internal/daemon/auth.go#admit
    anchor: "case !slices.Contains(roles, who.Role):"
  - actor: /components/daemon.md
    label: decode one bounded submission
    action: "`decodeSubmission` reads the body under a bound of 49 payloads plus 1 MiB (about 16 MB at the default `max_payload_bytes`), refuses unknown fields (a `source` key is a 400, the token names the source), requires 1 to 49 envelopes and a `definition_version`, and requires exactly one JSON value: anything after it is a 400, whitespace past the bound a 413. Nothing is queued yet."
    reads: [/data-structures/batch-submission.md]
    resource: internal/daemon/envelopes.go#decodeSubmission
    anchor: "dec.DisallowUnknownFields()"
  - actor: /components/daemon.md
    label: take a turn on the write queue
    action: "`tryWriteWithin` offers the job to the single write worker and waits up to `importer_wait` (5 s), or less when the request's budget is shorter, for it to be accepted; a mint or a long write such as a backup still running past that is a 503 with `Retry-After: 15` (`retry_after`), and a caller that has gone never starts the write. Once accepted the write runs on the daemon's context, so a client that disconnects does not cut it short."
    reads: [/data-structures/limits.md]
    resource: internal/daemon/queue.go#tryWriteWithin
    anchor: "case <-busy.C:"
  - actor: /components/gate.md
    caller: /components/daemon.md
    label: resolve binds the envelope to the person
    action: "A fresh gate is built for the batch with scopes `{bp-csv: [health.blood-pressure]}` and the registry snapshot read inside the write. `Resolve` validates the envelope and binds it to the configured person with assurance medium (no asserted person) or high (asserted and equal)."
    reads: [/data-structures/envelope.md]
    resource: internal/gate/gate.go#Resolve
    anchor: "case g.personID:"
  - actor: /components/gate.md
    caller: /components/daemon.md
    label: commit prepares every envelope first
    action: "`CommitBatch` refuses a batch over 49, then prepares each envelope and collects every failure into one `Refused` so the importer sees all reasons at once; two envelopes resolving to one facet are refused too. Any refusal writes nothing."
    resource: internal/gate/commit.go#CommitBatch
    anchor: "if len(envs) > MaxBatch {"
  - actor: /components/gate.md
    label: the three pillars without a ledger read
    action: "In order: envelope valid, binding is this person, source scoped for the type, Definition exists, version is the current one, assurance meets the Definition's floor, payload under `max_payload_bytes` (300 KiB), payload decodes with numbers kept as lexemes, closed JSON Schema passes. The first failure names the reason."
    reads: [/data-structures/definition-manifest.md]
    resource: internal/def/check.go#Check
    anchor: "if err := d.schema.Validate(doc); err != nil {"
  - actor: /components/gate.md
    label: fact-time, series, source_ref composed
    action: The fact-time pointer `/date_excel` is parsed with layout `2006-1-2 15:04:05` in `Australia/Sydney` and kept in UTC (2026-01-23T19:30:00Z). The series is empty for this type. The source ref composes from `identity_key` `[/date_excel, /reading]`, each value length-prefixed, giving `bp-csv:18:2026-1-24 06:30:00:1:1`; an envelope that carried a different one would be `ErrIdentityMismatch`.
    resource: internal/def/check.go#SourceRef
    anchor: 'return compose("identity", source, d.IdentityKey, doc)'
  - actor: /components/gate.md
    label: canonical bytes, content hash, facet id
    action: The payload is canonicalised (keys byte-sorted, whitespace stripped, number lexemes kept), hashed to `sha256:<hex>`, and the facet id is uuidv5 of the source ref under a fixed namespace. Identity says which fact; the hash says whether it changed.
    resource: internal/facet/payload.go#Canonicalize
    anchor: "sort.Strings(keys)"
  - actor: /components/store-ledger.md
    caller: /components/gate.md
    label: "one probe: admit, no-op or supersede"
    action: "`KnownFacets` asks which facet ids have a version 1; unknown ones admit at v1 with no further query. For a known one `CurrentVersion` reads the highest version by identity alone: same content hash is a no-op, a different hash supersedes at version+1."
    reads: [/data-structures/facet-artifact.md]
    resource: internal/store/ledger/read.go#CurrentVersion
    anchor: "ORDER BY artifact_version DESC LIMIT 1"
  - actor: /components/gate.md
    label: stamp anam-time and a seq block
    action: After reading the head W and deciding dispositions, the gate stamps one `anam_time` for the block and assigns seqs W+1..W+n in batch order, skipping no-ops. The stamp is inside the retry loop, so seq order and anam-time order agree by construction. A batch of only no-ops returns here without a transaction.
    resource: internal/gate/commit.go#CommitBatch
    anchor: "Seq:             int64(head) + int64(len(commits)) + 1,"
  - actor: /components/store-ledger.md
    caller: /components/gate.md
    label: artifact and event in one transaction
    action: "`AdmitBatch` checks the block is contiguous, opens an immediate transaction, re-reads the head and refuses a stale watermark (the gate re-reads and retries, up to `commit_retries`, five by default), classifies any existing `(facet_id, version)` as already-written (same bytes) or divergent (different bytes), then inserts the facet row with its payload as a BLOB and its admitted event. A trigger firing means the checks and the database disagree: `ErrCorrupt`."
    writes: [/data-structures/facet-artifact.md, /data-structures/anam-event.md]
    resource: internal/store/ledger/write.go#AdmitBatch
    anchor: "if err := checkBatch(readW, cs); err != nil {"
  - actor: /components/catchup.md
    caller: /components/daemon.md
    label: replay the new events into the index
    action: Still inside the write and only when something changed, `catchUp` runs `Sync` under the long-write deadline (`ingest_timeout`). It pins the ledger id, reads the index's last seq, replays the events after it, folds each touched facet forward with the same `fold.Apply`, and lands index rows and the cursor in one ops transaction. A failure here leaves the index behind and never fails the batch.
    reads: [/data-structures/anam-event.md]
    writes: [/data-structures/facet-index-row.md]
    resource: internal/catchup/catchup.go#Sync
    anchor: "events, err := l.Replay(ctx, ledger.Watermark(last), head)"
  - actor: /components/store-ops.md
    caller: /components/daemon.md
    label: record the snapshot part, when labelled
    action: A batch that carried a `snapshot` label (a `poll` run, not this `handle` run) has its part recorded after the ledger commit, in the other file, with every result's facet id including no-ops and the digest of its envelopes; a part resent with other envelopes is a 409 before anything is admitted, and the part that completes a run marks it complete.
    writes: [/data-structures/snapshot-part.md]
    resource: internal/store/ops/snapshot.go#RecordSnapshotPart
    anchor: "case recorded != p.Digest:"
  - actor: /components/daemon.md
    label: answer 200 and request a mint
    action: The response is `{w, changed, results[{index, outcome, facet_id, artifact_version}]}`, with `snapshot` progress when a part was recorded. A 422 lists every refused envelope; 409, 503, 504 and 500 carry one error. If anything changed the source is marked seen and a mint is requested on a one-slot channel.
    writes: [/data-structures/admission-response.md]
    resource: internal/daemon/envelopes.go#admissionOf
    anchor: "a := api.Admission{W: int64(head), Changed: changedBy(results), Results: make([]api.EnvelopeResult, len(results))}"
  - actor: /components/mint.md
    caller: /components/daemon.md
    label: mint after 2 s quiet, as its own write
    action: "`mintOnRequest` waits until no batch has arrived for 2 s, then `mintAtHead` takes its own turn on the queue and runs the incremental mint: the tail since the last run dirtied `bucket:2026-01`, so that bucket is rebuilt, hashed, written as a new checkpoint version, and the run sealed. A failed mint is only a log line here (see [mint failure after a batch is only logged](/gaps/mint-failure-after-batch-only-logged.md))."
    writes: [/data-structures/checkpoint-value.md, /data-structures/checkpoint-row.md]
    resource: internal/daemon/minting.go#mintAtHead
    anchor: "m, err = mint.Report(ctx, d.mintDeps(s), api.MintQuery{})"
  - actor: /components/importer-base.md
    label: accept only a whole admission receipt
    action: "`admission` checks the 200 body is a complete receipt: `w` a positive integer, `changed` equal to the admitted plus superseded count, exactly one result per envelope with a known outcome. Anything less is a retryable error and no admitted event is emitted. A 422 is `Refused` and not retried; 5xx, 429 and transport errors retry; 409 and other 4xx do not."
    reads: [/data-structures/admission-response.md]
    resource: ../anam-importers/anam_importer/client.py#admission
    anchor: 'OUTCOMES = {"admitted", "superseded", "no-op", "tombstoned"}'
  - actor: /components/anam-submit.md
    label: report admitted, alert on absent
    action: "`handle` emits `anam.admitted` with the outcome counts. When the batch completed a snapshot and the report says facets are absent, it also emits `anam.snapshot_absent` keyed on the snapshot id, for the gateway to tell the operator; nothing is retracted."
    ends: true
    resource: ../anam-importers/anam_submit/run.py#absent_event
    anchor: "dedupe_key=f\"anam.snapshot_absent:{snapshot['id']}\""
---

This is the path the whole design serves: a vendor-shaped row leaves the importer as a payload only, and everything that makes it a fact (identity, fact-time, series, hash, status) is decided at the gate from the Definition. Re-running any step is safe: the same export resends as no-op, a crashed batch is re-sent whole, and a crashed mint is caught up by the next one.

What the trace shows that the docs state in the abstract:

- The source is never a field. The importer's token names it, so an importer cannot write another source's facets.
- Two policies for `definition_version`: the daemon requires it, the gate would still accept 0 as unstated, a policy left from the in-process workers that no caller uses since card 119.
- The index and the snapshot part are written after the ledger commit, in the other file. A crash between them leaves them behind, and a resend repairs both.
- The importer trusts nothing short of a whole receipt, because a 200 with a truncated body once looked like success.

Related: [the gate composes identity](/decisions/the-gate-composes-identity.md), [commit is one transaction](/invariants/commit-is-idempotent-and-atomic.md), [importers live outside the binary](/decisions/importers-live-outside-the-binary.md).
