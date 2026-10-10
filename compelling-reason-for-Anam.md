# A compelling reason for Anam

**Give ten seconds of context today. Recover something valuable six months later.**

Personal data captures fragments of a life. A watch records a high heart rate. A rowing machine
displays a workout. A prescription records an instruction. What connects those fragments often lives
only in memory: you were presenting to a large team, you had just finished training, you started the
medication three days after it was prescribed.

Anam becomes compelling when contributing that context is as easy as sending a photo or saying a
sentence—and when the context remains available, attributable, and correctable years later.

A conversational assistant connected through MCP could make that possible. The assistant interprets
what you send, asks for missing information, and helps you investigate. Anam preserves the record,
its evidence, and the history of what changed. This is the intended experience, not a claim that all
of these capabilities are already implemented.

## Imagine it working, then work backwards

Six months from now, you are preparing for an appointment. You ask:

> “What was happening around those high-heart-rate alerts, and what medication was I taking then?”

The assistant retrieves the relevant history. Some alerts coincide with recorded exercise. One has
your note about presenting to a large team. Others have no recorded context. The medication account
distinguishes what was prescribed from what you said you started, changed, or stopped. You can open
the supporting documents and correct anything you remember differently.

The answer makes its limits visible. An alert with no context is unexplained; it is not automatically
an event at rest. A prescription is evidence of prescribing; it does not establish that you took the
medicine. A presentation coinciding with an alert does not establish its cause.

You leave with an account you can inspect and share, without reconstructing it from several apps,
photos, and uncertain recollections.

Work backwards from that moment:

1. **The answer needs retrievable evidence.** Every material statement must lead back to a recorded
   observation, your own account, or an explicitly identified interpretation.
2. **The record needs context across time.** Measurements, medication changes, exercise, and life
   events must be readable together, with event time separate from when Anam learned about them.
3. **The context needs to survive ordinary conversation.** A useful remark must become a durable
   record when you ask to save it, with confirmation of what was saved.
4. **Capture needs to fit into life.** Sending a photo or a sentence must usually be enough. Questions
   should resolve meaningful uncertainty rather than make you operate the underlying data model.

That future answer is earned by small contributions made when the context is still fresh.

## Stories that make the value concrete

### A photo after rowing

**User story:** As someone finishing a rowing session, I want to send a photo of the display, so I can
keep a useful workout history without transcribing numbers.

**Job story:** When I finish a workout and the results are still on the screen, I want to capture them
with one quick action, so I can leave knowing the session will be available later.

I send the photo and say, “Save today's row.” The assistant extracts the readable measurements,
shows what it understood, and asks about a digit or unit only if it matters and is unclear. It
preserves the image as supporting evidence. The event's time comes from the available context or
clarification; the upload time alone does not prove when the workout happened.

**Done means:** the session is saved once, the original image is accessible, uncertainty is visible,
and a correction does not erase the earlier reading.

### A prescription and what actually happened

**User story:** As a person managing medication over time, I want to send a prescription and record
when I start taking it, so I can later reconstruct both the instruction and my own account of use.

**Job story:** When I receive a new prescription, I want help recording the medication, dose, and
instructions while the document is in front of me, so I do not have to reconstruct them months later.

I send the prescription. The assistant reads the legible details and retains the document. It asks
whether I have started taking it if that information is needed. “I started on Monday” becomes a
separate assertion from the prescription. A later dose change or stop belongs to the same course
where appropriate.

**Done means:** “What was prescribed?” and “What did I report taking?” can produce different,
evidence-backed answers. A prescribed schedule never silently becomes a history of doses taken.

### The presentation behind the alert

**User story:** As someone reviewing a heart-rate alert, I want to add what I was doing, so the
measurement retains the circumstances I remember.

**Job story:** When I notice an alert whose context I still remember, I want to attach that context
quickly, so a future review does not leave me guessing about an isolated number.

I say, “Yesterday's high heart rate happened while I was presenting to a large team.” The assistant
finds the relevant event, clarifying which one if several match. It records my account and associates
it with that event. The watch's observation remains intact, and my explanation remains attributed
to me. “Yesterday” is resolved against the conversation's date and time zone, not a later retry.

**Done means:** a future reader can distinguish the measured event, my reported circumstances, and
any subsequent interpretation of their relationship.

### A question months later

**User story:** As someone trying to understand a period of my life, I want to ask questions across
my records in ordinary language, so I can investigate without remembering which system held each fact.

**Job story:** When an appointment or a recurring concern makes older events relevant, I want to
recover the surrounding evidence and gaps, so I can prepare an accurate account without starting
the search from scratch.

I ask, “Have those alerts mostly happened during presentations, exercise, or at rest?” The assistant
examines the recorded events and available context. It distinguishes supported associations from
unknown circumstances and explains which period and sources it examined. I can inspect the evidence
and retain the account that was shared.

**Done means:** the answer is traceable, its coverage is explicit, and missing observations are not
presented as evidence that nothing happened.

## What the MCP connection must enable

MCP provides a way for an assistant to use Anam. The practical value depends on the operations and
contracts exposed through it:

- Discover the record's supported types, definitions, projections, and available coverage.
- Preserve source images and documents, with durable links from extracted records to their evidence.
- Submit user assertions and extracted readings with their distinct provenance and review state.
- Confirm durable outcomes and retry safely without creating duplicates.
- Correct earlier submissions while retaining their history.
- Query event time and knowledge time, retrieve exact evidence, and publish reproducible views.

The assistant handles conversation and interpretation. Anam owns admission, identity, temporal
semantics, and the durable record. A conversation ending, a model changing, or a client restarting
must not make an acknowledged contribution disappear.

## Why the engineering matters to the person

Through **Hickey's structural lens**, the essential separations are observation from interpretation,
prescription from reported use, event time from knowledge time, and evidence from its presentation.
These distinctions let the record become richer without silently changing what an earlier source said.

Through **Armstrong's runtime lens**, the practical promise is that a saved contribution survives
interruption. An uncertain write must be reconcilable; an interrupted import must be repeatable;
an unavailable source must produce a visible gap or error. Recovery must preserve the evidence.

The lenses converge on a trustworthy record. Their tension appears at capture: preserving every
important distinction can create more questions than a person wants to answer. The design must keep
uncertainty explicit while making clarification proportional to its consequences. It must never
resolve that tension by inventing precision.

## The practical test

Use these stories in ordinary life for a month. Measure the effort to contribute, the corrections
needed, the evidence recovered, and the maintenance required. Include occasions when the assistant
cannot answer: honest gaps are part of the product's usefulness.

The strongest signal is a moment when you ask about something you would otherwise have forgotten,
and Anam recovers both the evidence and the sentence you contributed when you still remembered.

**Anam earns its place when the small effort of recording the present makes the past meaningfully
easier to understand.**
