---
type: workflow
title: Hand Anam a document
description: anam_documents names a scanned letter and its sidecar in a batch; anam_submit posts the bytes to POST /v1/documents, the daemon stores them in the library as <sha256>.<ext> through the write queue, and the doc.reference that names them is admitted only if the library holds bytes with its sha256.
timestamp: 2026-10-14T00:00:00Z
resource: internal/daemon/documents.go#postDocument
anchor: "stored.New, err = library.Store(d.cfg.LibraryDir, d.cfg.LibraryFolderLimit, stored.SHA256, ext, body)"
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
    anchor: "reads the share's `documents/` folder, and `anam_submit` posts each file's bytes to"
  - actor: /components/anam-submit.md
    caller: /components/importer-documents.md
    label: POST each file's bytes first
    action: "For each entry it posts the raw bytes to `POST /v1/documents?ext=<extension>` under the importer's token, at most `max_document_bytes` (64 MiB by default), before any envelope of the batch. In anam-importers `Client.store_document(name, data)` takes the extension from the name's suffix and leaves off one Anam would refuse."
    reads: [/data-structures/batch-event.md]
    resource: docs/IMPORTERS.md#8.4a Documents
    anchor: "`POST /v1/documents?ext=<extension>`, body the raw bytes (at most `max_document_bytes`, 64 MiB by default),"
  - actor: /components/daemon.md
    caller: /components/anam-submit.md
    label: check the token and the extension
    action: "`postDocument` refuses a token whose types do not include `doc.reference` (403) and an extension that is not letters and digits, at most 16 (400). It measures the sha256, size and media type (by extension) from the body."
    resource: internal/daemon/documents.go#postDocument
    anchor: "if !mayWrite(imp, docType) {"
  - actor: /components/daemon.md
    label: store the bytes as a value
    action: "On the write queue, so one writer counts and places and a backup never copies half a file, `library.Store` writes `<sha256>.<ext>` through a temporary file renamed into place under an `os.Root` on the library, in the first folder along the root, `sha[:1]/`, `sha[:1]/sha[:2]/` that holds fewer than `library_folder_limit` files. New bytes are 201 and bytes already held are 200, each with `sha256`, `bytes`, `media_type` and `new`. Other bytes are another file, so there is no conflict and nothing is overwritten."
    resource: internal/library/library.go#Store
    anchor: "name := path.Join(dir, sum+ext)"
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
    action: "Inside the write and before the gate, `checkReferences` indexes the library once with `library.Index` and looks up each `doc.reference`'s sha256; a reference whose sha256 the library holds no bytes for refuses the batch, and nothing of it is admitted. The path plays no part in finding the bytes."
    ends: true
    resource: internal/daemon/documents.go#checkReferences
    anchor: "post the bytes to /v1/documents first"
  - actor: /components/gate.md
    caller: /components/daemon.md
    branch: every reference names held bytes
    label: admit as any batch
    action: "The gate admits the references as in [admit a batch](/workflows/admit-a-batch.md). The `path` is the document's name and identity: the same file resent is a no-op, and a rescan under the same path with other bytes supersedes the facet while the first bytes stay in the library."
    ends: true
    resource: internal/daemon/envelopes.go#postEnvelopes
    anchor: "if err := checkReferences(d.cfg.LibraryDir, envs); err != nil {"
---

Before card 102 the library was a folder Anam's documents worker read in place, and something outside Anam put the files there. Now the bytes come in over the importer's token like the envelopes do, so the library has one writer and a reference can be checked against the bytes it names. Card 150 named each stored file by its sha256, so a changed document no longer needs a new path: its new bytes are a new file and its facet supersedes. Where a file sits is the [library](/components/library.md)'s own business and never enters a fact. The inspector serves a reference's bytes by sha256, confined to the library (see [inspector](/components/inspect.md)).

Related: [the Envelope is the only ingress](/invariants/envelope-is-the-only-ingress.md), [importers live outside the binary](/decisions/importers-live-outside-the-binary.md).
