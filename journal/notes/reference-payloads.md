# Reference payloads: design pass, 2026-10-07

Hickey × Armstrong, Before pass, on kanban card 24: the first use of the reserved `reference`
payload kind, a `doc.reference` facet for a file whose bytes stay in a source folder.

## The shape

- A Definition declares `payload_kind: reference` (default `verbatim`). The gate copies it onto the
  facet's descriptor, as it copies classification. That is the whole of the kind: a marker that the
  payload is a pointer whose target is not in Anam.
- The payload is the native reference, JSON like any other: `path`, `sha256`, `media_type`, `bytes`,
  `date`, and whatever the sidecar adds. The Definition requires the first five.
- Identity is the relative path (`documents:<path>`). The content hash is, as for every facet, the
  hash of the payload, and the payload carries the file's own sha256, so a file whose bytes change
  supersedes and a renamed file is a new facet. Both rules fall out of the existing gate; nothing
  new decides them.
- Fact-time is read from a sidecar, `<file>.meta.yaml`, with `date: YYYY-MM-DD`. A file with no
  sidecar is refused, and the message names the sidecar to write. EXIF is not read: that would be
  the bytes supplying their own fact-time, and it is also a dependency; the sidecar is the fact-time
  from outside the bytes that SELF-LOG.md §5 asks for.

## Hickey: what the lens changed

**The descriptor describes the payload, not the target.** The architecture reserved `media_type`
and `content_hash` on the descriptor "for the reference". Read literally, a reference facet's
descriptor would describe the bytes it does not hold, and `facet.New`'s invariant, that the hash is
over the payload it was given, would have to bend for one kind. Un-braided: the descriptor says the
payload is a JSON reference (`application/json`, `reference`), and the reference says what it points
at (`media_type`, `sha256`, `bytes`). One invariant for both kinds; the inspector reads the target's
type from the payload, which is where a pointer's description belongs.

**No new fact-time machinery.** The sidecar's `date` is the payload's `date`, read by the ordinary
`time_semantics` pointer. A worker that invented a second clock for files would be the kind of
special case the gate exists to refuse.

**The folder is a source like the others.** Same worker contract: read in the native format, emit
envelopes, write nothing. The worker reads the sidecar and the bytes (to hash them) and emits a
payload that is bounded in size whatever the file is. Anam owns no file.

## Armstrong: what the lens confirmed

**A refusal fails the scan, not the file.** As with records: one unreadable source ends the run with
the ledger never opened. So the live `imports.yaml` does not get a `documents` import until the
processed folder carries sidecars, or the hourly ingest would refuse every source. The handoff says
so.

**Hashing streams.** A scan reads each file once through `sha256`, never into memory whole.
`safepath.Open` resolves symlinks and refuses anything but a regular file, and the walk skips
dot-directories.

**A reference is bounded.** The ledger payload is a few hundred bytes however large the document;
the gate's size policy stands unchanged.

## Deferred

- Retraction when a file is removed (card 17).
- EXIF, if an image source ever needs its own date and nobody wants to write the sidecar.
- Serving the bytes through a projection. The inspector shows path and hash and nothing more.
