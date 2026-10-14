---
type: component
title: anam_documents
description: Sends a folder of documents with .meta.yaml sidecars as one snapshot of doc.reference envelopes, each batch naming the files whose bytes anam_submit hands Anam's library before the envelopes go.
timestamp: 2026-10-14T00:00:00Z
resource: docs/INBOX.md#Documents as facts of their own
anchor: "reads the share's `documents/` folder, and `anam_submit` posts each file's bytes to"
basis: docs
tier: detail
group: importers
depends_on:
  - /components/importer-base.md
  - /components/anam-submit.md
---

Anam's documents worker moved out (anam card 103), kept under the source name `documents` so a resend of the live library is all no-op (card 119). Every regular file under `documents_dir` is one document; dot-files, dot-directories and sidecars are skipped. Its fact-time comes from the sidecar `<file>.meta.yaml`, which must say `date: YYYY-MM-DD` and may say more (title, note); a file without one stops the run and names the sidecar to write. The sidecar is read with the [records importer](/components/importer-records.md)'s YAML reader, so both resolve a scalar as Go's yaml.v3 did.

Identity is the folder-relative path: a doc.reference's `path` is the document's name, and its `sha256` is how Anam finds the bytes, so a rescan under the same path supersedes the facet and the first bytes stay in the library. The importer measures `sha256`, `bytes` and `media_type` from the file, and a sidecar that declares any of them is refused. Each `anam_documents.batch` event carries at most 49 `doc.reference` envelopes, `current_versions`, and a `documents: [{path, file}]` list. [anam_submit](/components/anam-submit.md) reads each `file` and posts its bytes to `POST /v1/documents?ext=` before it posts the envelopes, so only Anam writes the library and no bytes ride in an event. Anam names the stored file `<sha256>.<ext>`; on anam-importers branch `card-150-sha256-library`, `Client.store_document(name, data)` takes the extension from the name's suffix (card 150). See [hand Anam a document](/workflows/hand-anam-a-document.md).

The plugin is on anam-importers `origin/master` (1dba240) and not in the checkout this bundle reads (see [importers checkout is behind origin](/gaps/importers-checkout-behind-origin.md)), so this page cites Anam's own account of it.
