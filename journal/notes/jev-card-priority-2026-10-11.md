# Jev classifies card priority

2026-10-11 · an experiment, not a decision. Companion to `jev-shaped-problems.md`.

**Jev agrees with the board on 20 of 33 open cards. Its disagreements are mostly upward, and
they come from reading each card's own case for itself. A card's `Why` section is written to
argue for the card, so a classifier over the card text rates rhetoric, not the owner's
sequencing. Confidence does not separate the agreements from the disagreements.**

## Setup

- Model `jev-1.13.0` (`jev-latest`), `POST /v1/systemone`, key from `~/.config/secrets/jev`.
- State: a five-line description of Anam, then one open card in full (frontmatter status and
  `blocked_by`, title, job story, why, acceptance criteria, narrative). Nothing else, so Jev
  does not see the other cards or the board's order.
- Two questions per card. A Choice over High, Normal, Low with one sentence of criteria each
  (Medium left out; the board uses it twice in 104 cards). A Score over four urgency levels.
- Each card asked twice with the Choice options reversed, to check order sensitivity.
- 66 calls, 97,886 input tokens, $0.0041, 8 in flight, about 25 s wall clock.
- Script and raw answers in the session scratchpad; the script is 110 lines of stdlib.

## Result

Agreement 20/33. Order-stable 33/33, the reversed options never changed the choice.

Confusion, board rows against Jev columns:

| Board \ Jev | High | Normal | Low |
|---|---|---|---|
| High (7) | 6 | 1 | 0 |
| Normal (18) | 6 | 10 | 2 |
| Low (7) | 1 | 2 | 4 |
| Medium (1) | 0 | 1 | 0 |

Nine of the thirteen disagreements rate the card higher than the board does.

The thirteen disagreements, with Jev's probability for its choice:

| Card | Board | Jev | p | Title |
|---|---|---|---|---|
| 55 | Normal | High | 0.98 | A Garmin importer, and the end of healthdb |
| 79 | Low | High | 0.94 | Name the Definition body a facet and a receipt rely on |
| 93 | Normal | High | 0.83 | Decide how a conversation writes to the record |
| 92 | Normal | High | 0.77 | Page a published read |
| 102 | Normal | High | 0.74 | Anam takes a document's bytes |
| 40 | Normal | High | 0.68 | Merge the health ETL branch |
| 91 | Normal | High | 0.45 | Read one facet by reference |
| 87 | High | Normal | 0.77 | Receipted reads over the API, and reads at a W |
| 41 | Low | Normal | 0.88 | Draw density at wide zoom |
| 36 | Low | Normal | 0.71 | Types for the glasses prescription and CPAP therapy |
| 32 | Medium | Normal | 0.50 | Upgrade GarminDB on unraid |
| 73 | Normal | Low | 0.58 | A proper README, with a luminary review |
| 37 | Normal | Low | 0.53 | Capture the 2023 medicines history |

Confidence is not a filter. Eleven agreements and seven disagreements have confidence at or
above 0.6. Cards 55 and 79 are Jev's two most confident answers overall and both disagree with
the board. The three disagreements near a coin toss (91, 32, 37) are the ones a threshold would
drop, and dropping them lifts agreement to 20 of 30, which is no better.

The urgency Score orders the board's High cards reasonably. Five of the seven are in Jev's top
fourteen by score; 89 (a reader token) and 87 (receipted reads) sit in the middle. The top of
the score list is 88, 92, 105, 55, 91, 30.

## Reading it

Where Jev is confidently wrong, the card argues well for itself. Card 79's job story asks to
trust a body "was not quietly changed under me" and its text says that if the operational
store is lost and a version is re-registered with another body "nothing notices". That is a
hazard by the criteria I gave. The board says Low because the hole is unguarded rather than
hit: every manifest so far bumps its version, and the store has not been lost. Card 55's text says healthdb ends when it lands; the board says Normal
because it is already in progress in the other stream and nothing is blocked on it. Card 87 is
High on the board because 91 and 92 wait on it, which the card's own text does not say; its
dependants' `blocked_by` lines say it, and Jev saw one card at a time.

So the question is answerable from the evidence I supplied, but the evidence is the wrong
evidence. Priority on this board is a function of the whole board (what is in progress, what
waits on what, what Matt is about to need) and of what the owner knows is not written down.
A classifier over the card's text answers "how important does this card say it is", which is a
different question, and one the author of the card already answered.

By the sorting test in the earlier note this is case 3, not case 2: ordering the board means
reconciling an open history, and the bounded version loses what matters. Two cheap things
would be fair to try before concluding more: supply the dependency graph in the state (the
whole board is about 40k tokens, under the 64k limit), and ask a Noul per card for "another open
card waits on this", which the frontmatter can check. Neither was run.

What this does support, cheaply: a lint that flags a card whose text argues High while its
frontmatter says Low, so the owner looks at it once. Card 79 is the example. That is a
reviewer's prompt, not a priority.

## What was not measured

One run, one prompt, criteria I wrote in a minute. No held-out set, no second labeller, and the
board's own priorities are one person's labels assigned over several weeks under changing
plans, so some of the "disagreements" may be the board being stale (32 is Medium on a board
that otherwise uses three levels). No comparison against a reasoning model given the same
input, which card 82's protocol would require.
