---
type: workflow
title: Hand Anam a document
description: anam_documents names a scanned letter and its sidecar in a batch; anam_submit posts the bytes to POST /v1/documents, the daemon stores them in the library through the write queue, and the doc.reference that names them is admitted only if the library holds those exact bytes.
timestamp: 2026-10-14T00:00:00Z
resource: internal/daemon/documents.go#postDocument
anchor: "stored.New, err = storeDocument(d.cfg.LibraryDir, rel, body, stored.SHA256)"
basis: code
tier: supporting
trigger: A folder watch on the documents folder runs `anam_documents handle` after `letters/2026-01-30-lipids.pdf` and its `.meta.yaml` sidecar land in it.
steps:
  - actor: /components/importer-documents.md
    caller: outside
    label: describe each file from its sidecar
    action: "Every file's sidecar gives its `date`; the importer measures the path, sha256, size and media type, and builds one `doc.reference` envelope per file. A missing sidecar stops the run."
    resource: docs/INBOX.md#Documents as facts of their own
    anchor: "Every file in the source folder has a sidecar beside it,"
  - actor: /components/importer-documents.md
    label: batch events name the files
    action: "Each `anam_documents.batch` event carries its envelopes and a `documents: [{path, file}]` list; the bytes stay on disk and are read only by anam_submit."
    writes: [/data-structures/batch-event.md]
    resource: docs/INBOX.md#Documents as facts of their own
    anchor: "`anam_submit` posts each file's bytes to `POST /v1/documents` before its"
  - actor: /components/anam-submit.md
    caller: /components/importer-documents.md
    label: POST each file's bytes first
    action: "For each entry it posts the raw bytes to `POST /v1/documents?path=<path>` under the importer's token, at most 64 MiB, before any envelope of the batch."
    reads: [/data-structures/batch-event.md]
    resource: docs/IMPORTERS.md#8.4a Documents
    anchor: "`POST /v1/documents?path=<library-relative path>`, body the raw bytes"
  - actor: /components/daemon.md
    caller: /components/anam-submit.md
    label: check the token and the path
    action: "`postDocument` refuses a token whose types do not include `doc.reference` (403) and a path that is absolute, climbs out, names a dot-file or a sidecar (400)."
    resource: internal/daemon/documents.go#postDocument
    anchor: "if !mayWrite(imp, docType) {"
  - actor: /components/daemon.md
    label: store the bytes as a value
    action: "On the write queue, so a backup never copies half a file, `storeDocument` writes through a temporary file renamed into place under an `os.Root` on the library. New bytes are 201, the same bytes again 200, other bytes at a held path 409: a changed document takes a new path."
    resource: internal/daemon/documents.go#storeDocument
    anchor: "a changed document takes a new path"
  - actor: /components/importer-base.md
    caller: /components/anam-submit.md
    label: POST the envelopes
    action: The batch goes to `POST /v1/envelopes` as any other.
    writes: [/data-structures/batch-submission.md]
    resource: ../anam-importers/anam_importer/client.py#submit
    anchor: 'status, body = self._call("POST", "/v1/envelopes", payload)'
  - actor: /components/daemon.md
    caller: /components/importer-base.md
    branch: a reference to bytes the library lacks
    label: refuse the whole batch, 422
    action: "Inside the write and before the gate, `checkReferences` reads each `doc.reference`'s path and sha256 against the library; any that do not match refuse the batch, and nothing of it is admitted."
    ends: true
    resource: internal/daemon/documents.go#checkReferences
    anchor: "post the bytes to /v1/documents first"
  - actor: /components/gate.md
    caller: /components/daemon.md
    branch: every reference names held bytes
    label: admit as any batch
    action: The gate admits the references as in [admit a batch](/workflows/admit-a-batch.md); the same file resent is a no-op.
    ends: true
    resource: internal/daemon/envelopes.go#postEnvelopes
    anchor: "if err := checkReferences(d.cfg.LibraryDir, envs); err != nil {"
---

Before card 102 the library was a folder Anam's documents worker read in place, and something outside Anam put the files there. Now the bytes come in over the importer's token like the envelopes do, so the library has one writer and a reference can be checked against the bytes it names. The inspector serves a reference's bytes from the library, confined to it (see [inspector](/components/inspect.md)).

Related: [the Envelope is the only ingress](/invariants/envelope-is-the-only-ingress.md), [importers live outside the binary](/decisions/importers-live-outside-the-binary.md).
