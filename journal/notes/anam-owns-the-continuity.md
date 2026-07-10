---
id: 202607100024
type: decision
title: "Anam owns no fact, and Anam owns the continuity"
created: 2026-07-10
status: evergreen
tags: [anam, decision, core, invariants, honesty]
project: anam
related: ["[[the-inversion]]", "[[verbatim-payload-metadata-separation]]", "[[ledger-and-canonical-two-surfaces]]"]
---

# Anam owns no fact, and Anam owns the continuity

**Decision.** Anam's founding invariant gets a second sentence.

> **Anam owns no source fact** — every captured payload is verbatim-from-source and rebuildable.
> **Anam owns the continuity** — the composed, ordered, whole-life account exists nowhere else.

## The challenge that forced it

Arch's ADR 0035 aims Anam's own invariant back at it, and lands:

> *"'Owns no facts, rebuildable from source' stays literally true under either grain — but under
> Person-grain it stops being the **whole** truth: the cross-episode continuity and identity linkage exist
> **nowhere else**, so REC becomes the de-facto record of *the relationships over time* even while owning
> no facts."*

It is right. "If Anam were deleted tomorrow, no fact would be lost" is **true and evasive**. Every
*payload* rebuilds by re-ingesting its source. The **continuity** — the ordering, the watermarks, the
account of what was known when, the assembled shape of a life — is authored by Anam and held nowhere
upstream. A health database does not know it is part of a life.

## Why the evasion is dangerous

"We own no facts" is the sentence that licenses everything else: it is why the whole-person read is safe,
why the substrate is swappable, why losing the store costs a re-ingest. Left unqualified, it also
licenses **not thinking about backups, retention, or what deletion means** — because supposedly nothing
is lost.

> A system that believes it holds nothing of its own will not protect the thing it holds.

That is precisely how a context product becomes a system of record: not by deciding to, but by never
noticing it already has.

## The distinction that keeps it honest

**Re-derivable** is not the same as **held elsewhere.**

The continuity is re-derivable: re-ingest every source and the timeline reassembles. It is not held
anywhere else while Anam is switched off. So it is **authored, backed up, never assumed rebuildable** —
the same status Anam already grants its checkpoints and receipts, now correctly extended to the thing
they are checkpoints *of*.

Anam is not weightless. It is a lens that, once ground, is the only one of its shape in existence. The
glass came from elsewhere; the grinding did not.
