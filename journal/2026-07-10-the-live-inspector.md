---
type: journal
date: 2026-07-10
session: 4 (continued)
title: "The live inspector — the diagram, drawn from the ledger"
tags: [anam, journal, inspect, fold, dataviz]
---

# The live inspector — the diagram, drawn from the ledger

The capture→compose→use diagram (`docs/assets/capture-compose-use.png`) stopped being an aspiration:
`anam inspect`'s landing page now draws its first two tiers from the live ledger, over the 659 facets
the big import admitted an hour earlier.

The screen is the **two-surfaces doctrine, drawn** (§7). The capture band is the **ledger**: every
facet as a diamond at its event-time, stems to a baseline, colored by facet type, the dead ghosted
with dashed stems — everything, marked. The composed strip below is the **canonical**: the live-only
value at `W`, in event-time order, each diamond a link to its facet. Drag a span on the band (or type
`from`/`to`) and the strip composes that span — labeled explicitly as *a lens, not a different read*,
with the header still naming the whole-person watermark and canonical hash. The third tier, *use*,
waits for projections (R7).

Decisions and discoveries:

- **The inspector graduated to the fold.** R3's discipline — show *last event*, never status, because
  the fold didn't exist to derive one — retired. The facet list and facet page now show folded status,
  from the same `fold.Read` the timeline uses.
- **A watermark-keyed value cache.** A whole-person read per page view was too dear; now every request
  reads the head (one cheap call) and reuses the cached `fold.Result` while `W` is unmoved. No TTL, no
  staleness: the cache holds a *value*, keyed by the watermark that makes it true.
- **The fold fetch went concurrent** (16 workers, bounded): 659 sequential `CurrentVersion` calls
  priced the read at minutes; parallel it is ~1.4s cold and ~30ms warm. Same error semantics —
  outpaced retries, corrupt stops.
- **Palette by the book:** four categorical slots in fixed order, validated for both modes with the
  dataviz checks (light yellow carries its contrast WARN via always-visible legend and mark rings).
  `fill:` styles SVG and `background-color:` styles HTML — one class sets both, each element listens
  to the one that applies.
- **Colliding marks step their stems.** Same lane + same event-time (the generic metric-group carries
  several groups per day) stacked invisibly; now each successive mark lifts one diamond-height,
  which is incidentally exactly how the hand-drawn diagram avoided the same problem.
- A shell lesson for the log: `pkill -f "anam inspect"` matches the *invoking shell's own command
  string* and kills it. `pkill -x anam` matches the process name.

## Carried forward

- The *use* tier (projections + receipts) joins the page at R7 — the diagram's third band.
- Band density at whole-person zoom is honest but heavy in the day-data months; bucket-level
  aggregation on the band (a count per week above some threshold) is a candidate if it ever matters.
- The inspector still re-fetches the dead individually per page; fine while the dead are few.
