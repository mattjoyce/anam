---
type: journal
date: 2026-10-17
tags: [anam, journal, public, location, stays, importers, meds, projection, definitions, datalog]
---

# 17 October: the repos go public, the places I stop, and what I was on in June

Two sessions worked this day. ThinkPad.anam1 finished card 142, and `mattjoyce/anam` became a
public Apache-2.0 repository with its history kept, a README written for a stranger, a NOTICE for
the IEEE 1752 schemas, the explainer bundle brought up to master and Anam's lineage moved out of
the front page into `docs/HISTORY.md`. `anam-mcp` followed at Matt's word (card 144), and card 171
asks for a drawing of how the pieces fit, for the README. The rest of the day was
ThinkPad.anam-157's: a new facet for the places Matt stops, Matt's supplements written into the
record by hand, and a bug found by reading them back.

## The places I stop

`location.track` (card 130) holds the phone's raw fixes and its own schema promises they are never
snapped, smoothed or joined to a place. Card 168 asked for the places anyway. Two shapes were
weighed and dropped: reverse geocoding inside the OwnTracks importer, which would put a network
call in the admission path of a GPS fix, and a place field superseded onto the track facet, which
loses a write war with an importer that resends each hour whole. The chosen shape is a sibling
facet, `location.stay`, from its own source, `osm`, keyed so a read can join it to the track
([[an-external-reading-of-a-fact-is-a-sibling-facet-from-its-own-source]]). The raw fix stays raw
at every watermark, and a geocoder that is down means places arrive late, nothing more.

The `anam_stays` importer went into anam-importers (2b5dfd2): it reads the spool the gateway pulls,
finds stays, geocodes their centroids through OSM with a cache, and never logs a coordinate.
ThinkPad.unraid wired it on the gateway the same afternoon (card 170): a principal and token for
`osm`, two instances, two pipelines, backups first. The first poll found five stays and then
stopped at the gate: the Definition's `place.address` was an open object, and since card 125 a
schema must close every object. One line fixed it (db55f00), and the second poll admitted the five
at W 33295, `osm` now a live source beside `garmin` and `owntracks`, Sydney Airport named by its
plaza. Card 169, the projection that folds places into the location timeline, Matt parked: live
with the stays for a few weeks first.

## What I was on in June

Matt's supplements had never been in the record. From Amazon order history they went in as
seventeen hand-written `med.*` rows on the Library share: started, dose changed, stopped and a
compliance estimate for each. Writing them with no guidance in the Definitions meant copying
conventions from the one other `med.*` file and guessing three things: how to normalise a
supplement's name when `drug` is half the `med.regimen` series key, what shape a self-log id
takes, and how to say a stop date is estimated when no field says so. All three pass the admission
test, and two writers guessing differently would split one regimen into two series.

That is card 166's case made concrete, and the card moved a long way in a day. Matt named the
Definition's third block `handling`, over `conduct` and `rules`; `kind` and `family`, which the
inspector needs, went to `meta` as what a type is rather than how it is handled; and the block is
loose, a place to look, not a vocabulary Anam or the importer base enforces
([[handling-is-where-to-look-not-a-vocabulary-anam-enforces]]). Read against the code the card is
a parser change, a 409, two read surfaces and a paragraph of docs, with the first block on
`med.started` v5. Matt wants it built next, before card 165, once the guidance text is agreed.

Reading the supplements back found card 172. `anam timeline --at 2025-06-15 --projection meds`
showed ramipril alone, when felodipine and a multivitamin were also running. The meds projection
is a snapshot, and a snapshot projection took its snapshot at the head and then applied the
template, so a course stopped after June was already gone before the cut. The fix is the order:
`serve.Derive` cuts the fold at `--at` and then applies the template, and `Run`, `Verify` and the
read view all go through it, so a receipt replays the read it names. Merged as 265c26c, with a
test that was red against the old order with the live symptom, "served 1 courses, want 2".

The same conversation drafted card 173, a Datalog query surface for agents. The supplement
questions ("what was I on, and when wasn't I") were joins across types along fact-time, answered
by hand, and card 172's bug was two pipeline steps in the wrong order. In a rule the bound is a
parameter, and the wrong order cannot be written. Matt widened it on the spot: any schema field
from the served Definition, not a declared subset. It is sequenced after 166.

## Hickey × Armstrong

**Hickey.** The day's two designs are the same move. A stay is an external reading of a fix, and a
model's reading of words (card 93) is an external reading of a note: each is its own facet from
its own source, and the thing read stays as its source said it. Card 172 was a complection of
order: "latest per series" and "cut at a date" are two folds, and the result depends on which runs
first.

**Armstrong.** The geocoder sits outside the error kernel. It failed, in a way (the open object),
and the failure was a 422 with `retry: false` and no data admitted; the gate did its job and the
fix was upstream. The first stays deploy was left running with a failing step rather than rolled
back, because the failure was terminal and harmless.

## Left open

- Card 166's guidance text for `med.started` v5, which Matt is to approve or correct before the
  build; then card 173.
- Card 172 live: unraid runs v0.327, nine commits behind.
- Card 169 and the other stays follow-ups, parked by Matt.
- Card 171, the diagram.
