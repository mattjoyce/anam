---
type: journal
date: 2026-10-23
tags: [anam, journal, garmin, heart-rate, ductile, projections, attention, workbench]
---

# 23 October: fourteen thousand hours, and a cap

Three deploys, the attention types live, and the first importer response too big for the
gateway to carry.

## Live in the morning

Cards 211, 221, 204 and 222 went out as v0.553: a query may carry its own projection, the
schemas live in embedded SQL, the daemon can hold a source's admission, and `mind.attention`
with its started and ended pair is bundled, with the Workbench entering all three. Card 148's
landing plan settled that an endorsement is an anam-event. Cards 223 and 224 followed as
v0.564. Card 227 was cut when the daemon tests began failing on a quiet machine: since card 222
there are enough bundled Definitions to guard at open that a cold start under the race detector
sits near the five-second request timeout.

## An hour of heart rate

Card 129, built by opencode in the importer and by a Sonnet agent in Anam: `health.heart-rate.hour`,
sixty slots a facet, one per minute, read from the watch's monitoring files rather than
GarminDB's rewritten copy. A rehearsal on a copy measured it first: 14,617 hours, the decode
650 seconds, reads one and a half to two and a half times slower for good, and one month of the
heart projection at 482 KB, over the default read bound. So the live window raised
`max_read_bytes` to 2 MiB first.

Then the gateway poll failed after ten minutes: `plugin stdout exceeded max size 10485760
bytes`. Ductile caps a plugin's output at 10 MiB, a constant, and the whole archive is 14 MB.
The backfill went round the gateway, the poll run by hand inside the container and a small
driver feeding the 367 batches to the submit plugin, W 64571 to 79080, the snapshot complete.
Card 229 holds what stands: every Sunday resend will fail the same way until the snapshot is
windowed, and the two-stage design went to opencode's queue.

## Projections for agents

Card 180 decided: the operator names a default projection and the daemon advertises it; an
agent reads it first and may read others. Card 228 gave the Workbench the daemon's version and a
page of the configured principals. Both built and live by the next morning as v0.574, with
`recent-life` as the default. Card 231 was split off for the types no projection served.

## Shearline

Card 223 had left a seam for a rule engine. Shearline, Matt's own Datalog, was assessed as the
policy engine and as the query engine; the one real change it would force is exact decimal
numbers. Card 230 holds it, in the backlog by Matt's word: other cards first.

## Hickey × Armstrong

**Hickey.** The hour facet is sixty values in one fact, nothing averaged, so a later reading can
ask what the watch saw. The cap is a place where the runner's buffer and the importer's snapshot
contract meet, and the honest fix is in the contract.

**Armstrong.** The backfill by hand is what disposable means: the gateway could not carry it,
the importer and Anam did not care, and the operator path cost one evening. The resend schedule
failing weekly is the cost left standing.

## Left open

- Card 229, the windowed snapshot.
- Card 227, the fixtures' timeout.
- Card 200 on hold while development produces many versions.
- The nine one-off backup copies the automated session was refused leave to delete.
