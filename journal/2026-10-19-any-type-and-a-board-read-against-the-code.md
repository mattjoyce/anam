---
type: journal
date: 2026-10-19
tags: [anam, journal, any-type, board, query, taxonomy, openapi]
---

# 19 October: any type, and a board read against the code

Two threads on a Sunday. Matt asked whether any facet could be added from a conversation, and
by the afternoon it could. And fifty open cards were read against the code for the first time in
a week.

## Any type, under an authorised token

Card 177. Matt's direction in one sentence: a fact he instructs under an authorised token is an
authoritative fact. That closed what the card had left open. The durable home is the ledger, not
a draft tier; the assurance is high because the fact carries `asserted_person`; identity needs
nothing new, since every id already carries its source. So a principal may declare
`any_type: true` in place of a types list, and anam-mcp gained `save_fact` beside `save_note`: a
facet of any type the daemon knows, validated by the gate like an importer's batch.

The audit through [[guiding-lens-hickey-armstrong]] found the two edges to close before it
shipped. An any-type principal does not reach the library, so document bytes still need a
principal that owns `record.document`; and no write binds without a person, so `save_note`
sends `asserted_person` too. The first fact went in that evening from a chat on v0.347.

## Fifty cards, three agents

Three agents read every open card against the code. Seven closed as done in effect, each with
a line naming what delivered it. Seven were corrected: card 49 retitled to the hand-written
OpenAPI spec the Huma decision implied, 73 narrowed to its four remaining README items, 156
given a timeout name that does not collide with the HTTP one, 162 split into the dating half
already in code and the remote half that is not. The corrected ones were then built:
`long_write_timeout` with the old name as an alias, the README reviewed by a three-lens panel
with a dead-link check and a Definition-table test in the gate and a CHANGELOG begun,
`openapi.yaml` served and held to the mux by a test, and the glasses prescription bundled as a
type because its values were already transcribed in the library. Matt put card 32, the GarminDB
upgrade, on hold in plain words.

## The query and the taxonomy

Card 173's doctrine amendment, written in Matt's session, was reviewed and merged with two
fixes. The engine survey settled the method: prototype the questions in Soufflé, ship a
hand-rolled evaluator, keep Soufflé as an oracle. The first slice merged that night, and its
equality check agreed with the fold at all 4,135 fact-times of the 8 October copy.

Card 147 took its shape: eight domains, four live and four reserved, all 35 types mapped to
`domain.concept` with a family and a kind, and an order of operation with three gates for Matt.
The first draft moved the record into a new ledger. Matt asked why not supersedence, and whether
that keeps anam time; the schema said yes, the type is a column on each version row. So the
rename became an in-place run in one ledger, and card 140 became that run. Two cards cut from
the day: a single-dose medication type (179), since a dose taken at a moment is not a regimen,
and a default projection an agent reads first (180), drafted from the Mac.

## Hickey × Armstrong

**Hickey.** `any_type` un-braids who may write from what shape they may write. The shape stays
with the Definition and the gate; the principal says only who. The braid stays on purpose at the
library, where owning the document type is what admits bytes.

**Armstrong.** Nothing in the kernel changed. The board review was recovery for a different
drift, cards that no longer described the code, and three readers in parallel were cheap enough
to do again.

## Left open

- Card 147's three gates, with card 140 built ahead of the second.
- Card 162's remote half, waiting on Matt.
- Card 181: three findings the newest gosec raises and the ThinkPad's does not.
