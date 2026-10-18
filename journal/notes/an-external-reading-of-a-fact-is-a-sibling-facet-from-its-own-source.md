---
id: 202610171500
type: decision
title: "An external reading of a fact is a sibling facet from its own source"
created: 2026-10-17
status: evergreen
tags: [anam, decision, location, importers, sources, identity]
project: anam
related: ["[[verbatim-payload-metadata-separation]]", "[[a-persons-words-are-a-note-and-a-models-reading-is-a-draft]]", "[[one-owner-per-definition]]", "[[sources-reach-anam-only-through-importers]]"]
---

# An external reading of a fact is a sibling facet from its own source

When something outside the record interprets a fact (a geocoder naming the place a GPS fix was
taken, a model reading a note), the interpretation is written as its own facet, from its own
source, keyed so a read can join it to the fact it reads. The fact stays as its source said it.

Card 168 is the case. `location.track` holds the phone's raw fixes and promises they are never
snapped, smoothed or joined to a place. The places Matt stops are `location.stay`, from source
`osm`, derived by a separate importer that reads the same spool, finds stays and reverse-geocodes
their centroids. Card 93 made the same move for words: the note is what Matt said, and a model's
reading is a draft from source `interpretation`.

## The tension

A place on the track facet is what a reader wants, in one payload. Two things pull the other way.
The raw importer resends an hour whole whenever a new fix arrives and on every snapshot, so a field
added by anyone else is clobbered within a day; one slot with two writers is a place, not a value.
And a geocoder is a network call that can fail or lie, which must never sit in the admission path
of a fix: GPS recording does not wait on OSM.

## What it buys

- The raw fact is recoverable at every watermark, unchanged by what was made of it.
- The reading can be wrong and corrected without touching the fact, and its source says who read.
- The external call sits outside the error kernel. Down means late, not lost.
- A reading is not a pure fold over the ledger (it needs the network), so it genuinely enters as a
  fact through an importer, not as a projection.

## Rejected

- **Reverse geocode inside the OwnTracks importer.** Breaks that importer's contract (no network,
  no state) and puts a flaky call in the fix's admission path.
- **Supersede the track facet with a place field.** Loses the write war with the raw resend, and
  braids what the phone said with what OSM guessed.
- **A projection that geocodes at read time.** Not a fold; the answer would change with the
  network, not with the ledger.
