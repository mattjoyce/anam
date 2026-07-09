# Anam — PR/FAQ

> What Anam is and why, for a reader who isn't building it. A press-release-style framing followed by
> the honest questions. Companion to `ARCHITECTURE.md` (how).

---

## Press release

**Anam gives one person a single, trustworthy, whole-life timeline of themselves — assembled from the
systems that already hold their data, owning none of it.**

Your data about yourself is scattered: a watch measures your sleep and heart, a scale measures your
body, a cuff measures your blood pressure, and each lives in its own silo with its own app. No single
place shows you *you*, over time, with the evidence attached — and nothing you'd trust to tell you *what
it knew, and when*.

Anam is that place. It reads your sources, holds each fragment **verbatim** as a **Facet** — a fact plus
where it came from and when — and lays them on **one timeline**. You read your whole life by default;
you can focus on a **span** (a trip, an illness, a training block) when you want to, but you never have
to. Every fact is immutable and accretes; nothing is ever overwritten, so the past stays exactly as it
was recorded. When Anam or an agent notices something — "your resting heart rate ran high the week you
skipped your medication" — that observation enters as a *proposed* note beside the facts, never silently
rewriting them. And any view Anam gives you comes with a **receipt**: proof of exactly what you were
shown, at exactly what moment of knowledge.

If Anam vanished tomorrow, you'd lose nothing: it owns no facts and rebuilds itself from your sources.
It is a **lens on your data, not a second copy pretending to be the original.**

Anam is the soul-record (*anam* is Irish for *soul*) — built for one person, extensible to every facet
of their life.

---

## FAQ

**Isn't this just a health dashboard?**
No. A dashboard shows the *current* numbers; Anam is a **ledger of facts over time** with provenance,
immutability, and receipts. It can tell you not only "what is my blood pressure" but "what did I know
about my blood pressure on this date, and where did each reading come from." Health is simply the first
kind of facet; calendar, mail, and notes are the same shape.

**Where does Anam sit relative to Arch?**
Anam is a re-instantiation of **Arch**, Matt's person-centric clinical context architecture, with **one
decision inverted.** Arch deliberately *refuses* to be a longitudinal record — every read is scoped to a
bounded episode, so it can never become a system of record for a whole life. Anam wants the opposite:
the **whole-person timeline is the default read.** Everything else — the Facet, the append-only ledger,
the admission gate, the two clocks, minting, projections, receipts — carries straight over.

**Does Anam become a second copy of my life — a new thing to secure and trust?**
It holds your source fragments verbatim, so yes, it stores data — but it is **authored, never
authoritative.** It owns no fact; the sources remain the truth; Anam rebuilds from them. The line that
keeps it a *context product* and not a shadow record is structural: verbatim-from-source, append-only,
and rebuildable. (Long-term retention and erasure are a real, deferred question — see
`ARCHITECTURE.md` §13.)

**Can I trust what an AI agent adds?**
Agent findings — *insights* — enter as **proposed** facts through the same gate as everything else,
carrying which model produced them and from which snapshot. **Admission is not endorsement**, and
endorsing or rejecting an insight **never writes back** to a source. The agentic part is contained; it
can observe your facts but never rewrite them.

**How is "what I was shown" provable?**
Every served view is a **projection** — a template that only *prunes* facts, never reshapes or invents
them — and it comes with a **receipt**: the exact set of facts, the template version, and the *watermark*
(the moment of knowledge) it was rendered at. The same receipt reproduces the same view.

**Why immutable, append-only? Isn't that wasteful?**
Because a soul-record whose past can be silently rewritten is worth nothing. Immutability is what lets
Anam answer "what did I know, and when," lets a crashed import simply be re-run without corruption, and
lets the whole store be rebuilt from source. (Growth is an accepted, deferred cost — Anam owns no facts,
so an archive tier is always an option.)

**What's actually built first?**
The thin end-to-end slice over two real sources — a health database and a blood-pressure export —
proving capture → ingest → ledger → mint → fold → projection → receipt, with a CLI to read your
timeline. See `MVP.md`.

**What guides the engineering?**
Matt's Go standards, and a standing design lens — **Hickey × Armstrong** (*structure × runtime*):
keep the structure simple and un-braided (immutable facts, current state derived not stored), and keep
the runtime supervised (a small error kernel that can't corrupt, everything else disposable and
rebuildable). See `AGENTS.md`.
