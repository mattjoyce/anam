---
id: 202607102352
type: decision
title: "Publication is a receipt, not a record-event — and so W ≡ seq"
created: 2026-07-10
status: evergreen
tags: [anam, decision, record-events, watermark, receipts, publication]
project: anam
related: ["[[ledger-and-canonical-two-surfaces]]", "[[state-is-checkpoint-plus-replay]]", "[[doctrine-aphorisms-outlive-their-reasoning]]"]
---

# Publication is a receipt, not a record-event — and so W ≡ seq

**Decision.** There are exactly five record-event kinds — `admitted`, `superseded`, `tombstoned`,
`endorsed`, `rejected` — one per derivable status. `published` is deleted. A facet's published time is
the earliest `served_at` over the receipts naming it.

## The tension that exposed it

Three documents disagreed, and one counter cannot satisfy all three: §7 said `W` advances on five kinds
(`published` absent); §8 said `seq` numbers *every* record-event and `seq` **is** `W`; the glossary
listed `published` as a kind and omitted `rejected`. The code defined all six.

## Why deletion, not reconciliation

A record-event asserts that *Anam's knowledge of a fact changed*. Serving a facet changes nothing Anam
knows — it records what Anam **showed**, and that is what a receipt already is (served hash, facet-id
set, watermark, template version, span). Keeping `published` in the ledger braids *what Anam knows* with
*what Anam has shown* — the same complecting the ledger/canonical split had just untangled
([[ledger-and-canonical-two-surfaces]]).

With it gone, every remaining kind can change the live-set, so **`W` is the record-event sequence
itself**: one counter, no special cases, and the five kinds map one-to-one onto the five statuses.

## The cost, taken knowingly

Published-time moves out of the ledger into the receipts — the published-artifact tier, which is
authored: backed up, not rebuilt. Lose the artifact directory and published-time is gone for good.
Acceptable, because being shown is a fact about a *serve*, not about the fact.
