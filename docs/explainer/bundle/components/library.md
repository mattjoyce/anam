---
type: component
title: Library (internal/library)
description: "Anam's store of document bytes: files named <sha256>.<ext>, written only by Anam, placed by a folder limit and found again by a walk for the name."
timestamp: 2026-10-14T00:00:00Z
resource: internal/library/library.go#Store
anchor: "func Store(library string, limit int, sum, ext string, body []byte) (bool, error) {"
basis: code
tier: detail
group: published
depends_on: []
---

A document's bytes, as `POST /v1/documents` hands them in (cards 102, 150). `Store` names the file `<sha256><ext>`, writes it to a nonce-named temporary file with `O_EXCL` under an `os.Root` on `library_dir`, syncs, and renames it into place. Bytes the library already holds, under any extension, are not written again, so the answer is only whether they were new. Other bytes are another name, so a stored file is never overwritten or renamed.

Placement: a new file goes in the library root until the root holds `library_folder_limit` files (default 1000, `ANAM_LIBRARY_FOLDER_LIMIT`), then into the subfolder `sha[:1]/`, then `sha[:1]/sha[:2]/`, and so on. Only regular files that are not dot-files count; subfolders do not. A stored file never moves, so where it sits depends on when it came, and nothing records it.

`Find` and `Index` look a file up by its name: one walk of the library, skipping dot-files and dot-directories (temporaries on their way in) and any file not named by a sha256. `IsSHA256` is the lower-case hex test both callers use. The [daemon](/components/daemon.md) calls `Store` from `postDocument` on the write queue and `Index` from `checkReferences`; the [inspector](/components/inspect.md) calls `Find` to serve `/document?sha256=`. [Backup](/components/backup.md) copies the directory whole. See [hand Anam a document](/workflows/hand-anam-a-document.md).
