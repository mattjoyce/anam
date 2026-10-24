---
type: journal
date: 2026-10-22
tags: [anam, journal, schema, checkpoints, receipts, policy, mind, backups, opencode]
---

# 22 October: schema nine, a policy package, and a ninth head

Fifty-eight board lines, four deploys, a new coding agent, and a rule Matt gave about risk that
reshaped the day's advice.

## Checkpoints are a cache

Card 215 moved artifacts into the operational store as rows, and the question it raised was
whether checkpoints are part of the record. Matt held the card for discussion, measured the
timings on unraid, and decided: checkpoints are a cache that may go missing. The branch was
reworked to that and the store went to schema 9 in a window of its own, the old store moved
aside, a fresh init, artifacts moved in. Card 216 followed inside the same window: a token can
always leave receipts, and a receipt names its reader, so a recording token lists and verifies
only its own. v0.528.

The morning window before it had shipped the link form (202), one fact a day for heart rate in
place of three (205, with 1,936 old facts retired and 646 new), and selections with short codes
(206), as v0.494. Card 200's migrate carried every live facet on a copy once Matt decided the
1,600 weights admitted under an unparseable first version are carried under the current one
when their payload passes.

## A policy package, static for now

Card 223 came out of the audit fixes (224) and the receipts' two design calls. The permission
rules were scattered across handlers; now they are one package, plain Go, answering who may do
what with the same questions a Datalog could answer later. The explainer calls it build free,
structured for strict: the seam is drawn, the engine is not chosen. Card 221 put the schemas
and migrations in embedded `.sql` files, which Codex reviewed under the lens when the Fable
review ran out of credits.

## Not a hospital

Matt, on advice that had grown cautious: Anam is not a hospital. Size the risk to a personal
record, do the simplest thing that works, and say a caveat once. It changed the day's
recommendations, and it is written down as [[size-the-risk-to-a-personal-record]].

## A ninth head

LifeOS drafted card 222 from Matt: self-reported attention, what is absorbing him at a moment.
No existing domain fit. Matt decided a ninth head, `mind`, for self-reported inner states, with
attention as its first concept: a reading, and a started and ended pair the person declares.
[[a-domain-head-is-added-by-decision-not-earned]] records why that is a decision and not a
pattern.

## Opencode arrives

Matt set up opencode on the Mac as a coding agent. Its first report found six live types no read
path reached whole and two query bugs, a container pointer that could never bind and numbers
rounded through a float, both fixed and merged as PRs 5 and 6, then the gosec taint rules (8)
and the admission hold (9). A pi coder on the ThinkPad was tried on the lab's faux record and
abandoned the same day. A reader principal for LifeOS went live, and card 219 settled the backup
regime at three local dailies and three offsite.

## Hickey × Armstrong

**Hickey.** The policy package is the un-braiding of the week: authorisation as a value asked
of one place, not a condition repeated in each handler. Checkpoints as a cache is the same
clarity about what is derived.

**Armstrong.** The schema 9 window was the first that could not be a plain deploy, and the
runbook now says so in a hazard line before the window, not after. A store that is behind,
never wrong, is what made it safe to move the old one aside.

## Left open

- Card 217, Definitions and projections as ledger facts: Matt leans to the body as a document
  and the hash as a fact; not decided.
- Card 210, the faux record, back in the queue.
- Cards 211 to 213 from the query feedback; 211 built, the other two waiting.
