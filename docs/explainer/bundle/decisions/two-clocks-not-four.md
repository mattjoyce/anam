---
type: decision
title: Two clocks, not four
description: A facet carries fact-time and nothing else; an anam-event carries anam-time and nothing else; recorded-time and ingested-time were deleted as duplicates.
timestamp: 2026-10-11T00:00:00Z
resource: internal/clock/clock.go#Layout
anchor: 'const Layout = "2006-01-02T15:04:05.000000000Z07:00"'
basis: docs
tier: supporting
alternatives:
  - Four clocks, with a recorded time (when the source documented it) and an ingested time (when Anam received it) on the facet (built, then deleted: the first was a duplicate column across 893 rows, the second a second home for the admitted event's anam-time)
  - A published time as a third clock on the ledger (refused; it is derived from receipts)
affects:
  - /terms/fact-time.md
  - /terms/anam-time.md
  - /data-structures/facet-artifact.md
  - /data-structures/anam-event.md
---

**Context.** Telling "when was this true in the world" from "when did Anam learn it" is the difference between history and audit.

**Choice.** The split is structural, not conventional. A facet has one clock, fact-time, read from the payload by the Definition's pointer; an anam-event has one clock, anam-time, stamped by the gate. Neither can borrow the other's because neither has a field for it. Times are text in one fixed-width layout, so they compare lexically in chronological order; `time.RFC3339Nano` is explicitly refused because it elides trailing zeros.

**Why.** Every extra clock was a second place for the same answer to disagree from. Where the facet page once read ingested time off the artifact, it now reads it off the event that admitted that version.

**What it costs.** Anam-time has resolution but a bulk backfill compresses its span, so an as-of inside an import returns a read-order artefact. That is honest: Anam did not exist then. ARCHITECTURE §8 still calls the layout RFC3339Nano, which the code refuses (see [architecture doc names the wrong time layout](/gaps/architecture-doc-stale-sentences.md)).
