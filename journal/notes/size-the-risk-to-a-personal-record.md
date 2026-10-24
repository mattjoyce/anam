---
id: 202610221200
type: decision
title: "Size the risk to a personal record; Anam is not a hospital"
created: 2026-10-22
status: evergreen
tags: [anam, decision, risk, operations, agents, ductile]
project: anam
related: ["[[doctrine-aphorisms-outlive-their-reasoning]]", "[[a-hard-coded-value-is-a-default-an-invariant-or-presentation]]", "[[brooks-below-the-gate-beck-above-it]]"]
---

# Size the risk to a personal record; Anam is not a hospital

**Decision (Matt, 2026-10-22).** Anam is one person's record on one box. Advice, design and
operating procedure are sized to that: the simplest thing that works, a caveat said once, no
tier of defence for a threat the record does not have.

## The tension

The genesis drawing was a hospital, and the words around Anam, ledger, gate, receipts, tokens,
pull every agent toward the general case. Each control an agent proposes is defensible alone: a
schema migration window with a moved-aside store, a per-principal receipt policy, a backup
regime with offsite copies. Together they make a one-line change into a ceremony, and the
ceremony is what the operator pays for every week.

The other side is real: the ledger is the error kernel and must not corrupt, and some controls
are cheap and already built. The decision is not to remove them. It is to ask of each one who it
protects and from what, and to stop adding ones that answer "nobody, here".

## Where it bit

Ductile is the clearest case. Attestation, service accounts and the config model grew strict
because every agent that worked on it, Claude and GPT alike, saw a gateway with secrets and
designed for a platform. Matt's description of it is a runner: someone changes a plugin and it
should run. The shape agreed on 24 October is strictness opt-in per plugin root, with a
time-boxed clearpass that records the operator's intent before a change rather than a ceremony
after it, and loud refusals outside the window instead of a plugin that quietly stops spawning.

## How to apply it

When advising, name the risk in one sentence and the simplest fix; say the caveat once and do
not repeat it. When an agent proposes a control, ask what threat it stops on this box. Put the
sentence where new agents read first, because every new session rediscovers the enterprise
shape unless told.
