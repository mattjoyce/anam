# The inspector, reviewed (card 71)

Written 2026-10-10, after card 71's three slices landed (the Definitions pages, field
descriptions, the facet page by field and by version) and before any further change. It draws on
the Tufte × Majors pass in `luminary-definitions-page-2026-10-10/tufte-majors.md`, phone renders of
every page at 390 px, and page sizes measured on a 234-facet record and scaled to the live one
(W 20,951 on 2026-10-10).

The rule it holds to is Matt's, from card 65: the inspector is a diagnostic tool and yields to the
design. Nothing below asks the core for a store, index or read path.

## Page by page

| Page | The question it answers | Verdict | On a phone |
|---|---|---|---|
| `/` timeline | What the record holds, where in time, at what status | Stays, the landing page | Usable controls; the band is too small to read |
| `/facets` | Every facet in one table | Replace with a filtered, bounded list | Scrolls sideways (fixed b178965) |
| `/facet` | What this fact is, what it means, what happened to it | Done in card 71 | Reads well; fields stack |
| `/events` | The ledger in seq order | Keep, bounded and newest first | Scrolls sideways |
| `/projection` | What a template serves, as of an anam-time | Stays | Scrolls sideways |
| `/definitions`, `/definition` | What each type's contract and fields mean | New in card 71 | Reads well; fields stack |
| `/document` | A library file's bytes | Plumbing, stays out of the nav | n/a |

### Timeline

The one page that shows the whole record in time, and the right landing page. Two things against
it. At phone width the band is 390 px for the whole record, so marks are a couple of pixels and
the rows run together; that is card 41's problem (density at wide zoom) seen from the other side,
and it strengthens that card rather than needing a new one. And the band sets
`touch-action: none` so drag-to-zoom works, which on a phone means a vertical swipe that starts on
the band does not scroll the page. A narrow band with `touch-action: pan-y` would keep both.

Card 21 (checkpoints in the inspector) belongs here, as a strip under the band, not on its own
page. It stays standing.

### Facets

One unpaged table of every facet. At the live record that is about 7.9 MB of HTML (377 bytes a
row), and no question is answered by scrolling 21,000 rows. What people come for is "the facets of
this type", "the tombstoned ones", "what this source sent": a list filtered by `?type=`,
`?status=` and `?source=`, newest first, bounded, with the count of what matched. The timeline and
the Definitions pages already link to `/?type=`; they would link here as well. The filter reads
the fold the inspector already caches, so it adds no read path.

### Facet

Card 71 made it the page the job story starts on: the payload read through its admitted
Definition, the words in force when it was admitted, any artifact version by `?v=`, and what each
version changed. Its event list still replays the whole ledger to find one facet's events; the
gate has a per-facet read since card 65 (`Ledger.FacetEvents`), but by Matt's rule the inspector
does not need it, and the cached fold is what it reads. Left as is.

### Events

The raw ledger tail, useful when auditing what an ingest did. Unbounded it is about 6.5 MB at the
live record. Newest first, the latest few hundred by default with an older link, each row linking
to `/facet?id=&v=`, which slice 3 made possible.

### Projection

What a role would see, wound back to an as-of time. It is the one page with a free time control and
it reads as intended. No change.

### Definitions

New in card 71. The gaps it shows are the ones card 84 settles.

## What is missing

- **Who registered a Definition.** The operational store records when, not which importer; the page
  shows "bundled" or "registered". Each facet's source names the importer, which answers the real
  question ("who sends this type"). Not worth a column in the store.
- **A way from a type to its facts as a list**, the filtered `/facets` above.

## What Matt does not use

Only Matt can say. Two questions for him:

1. Do you open `/events` at all, or is the facet page's own event list enough? If not, it can go
   instead of being bounded.
2. Do you use the projection page's as-of control, or only the template switch?

## What follows

- A card for the facet and event lists: filtered and bounded `/facets`, bounded newest-first
  `/events`, and the band's `touch-action`. The answers to the two questions above shape it.
- Cards 21 and 41 stand as they are; this review adds the phone reading to 41.
