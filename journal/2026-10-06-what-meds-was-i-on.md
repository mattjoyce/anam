---
type: journal
date: 2026-10-06
session: 12
title: "What meds was I on, as of: the self-log, documents, and the snapshot read"
tags: [anam, journal, self-log, medication, snapshot, series-key, reference, design]
---

# What meds was I on, as of: the self-log, documents, and the snapshot read

Same evening as [[2026-10-06-r8-the-mint]], after the first mint ran and the inspector was up over
an SSH tunnel. Matt named hard and soft checkpoints ([[hard-and-soft-checkpoints]]), then brought the
next data: photos and images with extracted data, old blood tests as text files, prescriptions, a
self-estimated compliance figure, and later blood donations, being unwell, hangovers, international
travel, gym visits. The design is written up in `docs/SELF-LOG.md`; this is the story.

## The question

*What meds was I on, as-of?* It sounds like a read Anam has. It is not. `--from/--to` gives every
fact in a window; `--as-of` winds Anam's clock. Neither gives *the latest state of each thing at a
date*, and §4 of the architecture has said since July that this last-known-value read is not built.

## What fell out

**An interval is a series of point facts.** A regimen stored as one fact dated at its start is
invisible to a query in its middle, because fact-time is the only time a read may use. So started,
changed, stopped, each a point fact, each in the type so no read opens a payload. The series key
(card 15, open since July) is what groups them, and a Definition declares which type closes a
series, so the rule is data.

**Checkpoints do not look backward.** Matt asked whether a checkpoint would find a prescription that
covered its day. No, and it should not: a checkpoint is one bucket's live facets. The snapshot
composes the prefix of hard checkpoints up to D and folds per series over the union. A carry-in at
each bucket boundary would be faster and would make every later bucket depend on every earlier one,
the dependency fact-time bucketing was chosen to avoid. Measure first.

**Probable doses are not stored.** The one proposal that did not fit: seed daily dose events with
a probability from the compliance estimate. Those facts carry nothing beyond the two they derive
from. The schedule is a soft derivation, same word as the checkpoint. The regimen and the estimate
are facts, an observed or asserted dose is a fact, a probability is not.

**Documents are references; extraction is proposed.** The reserved `reference` payload kind gets
its first use. Bytes stay in the source folder, identity is the path, fact-time comes from outside
the bytes. What a model reads off a scan re-enters as a derived facet, `proposed`, endorsed by Matt,
superseded by a better extractor.

## Settled vocabulary

The source you keep by hand is the **self-log**: one file, one row per event, a row id you assign
as identity. Types are namespaced by subject: `med`, `lab`, `doc`, `donation`, `illness`, `travel`,
`gym`. A hangover is `illness.hangover`, a point fact. Detailed gym data a device records stays with
the device; `gym.visit` is your word that you went.

## Where it leaves us

Seven cards, ordered: series key (15), self-log source (22), medication types and snapshot read
(23), life events (27), reference payload kind (24), lab results (25), compliance estimates (26).
None touches the ledger, the gate, the fold, or the checkpoint. The test that defines done is the
same equality oracle R8 used: snapshot from composed checkpoints equals snapshot from a full fold
equals the answer worked by hand.
