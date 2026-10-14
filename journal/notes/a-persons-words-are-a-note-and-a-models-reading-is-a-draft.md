---
id: 202610151200
type: decision
title: "A person's words are a note, and a model's reading of them is a draft the person endorses"
created: 2026-10-15
status: evergreen
tags: [anam, decision, conversation, notes, interpretation, standing, mcp]
project: anam
related: ["[[sources-reach-anam-only-through-importers]]", "[[verbatim-payload-metadata-separation]]", "[[tombstone-is-a-record-event]]", "[[guiding-lens-hickey-armstrong]]"]
---

# A person's words are a note, and a model's reading of them is a draft the person endorses

**Decision (Matt, 2026-10-15, card 93).** When Matt tells an assistant something about his day, his
words are saved at once, verbatim, as a note under source `conversation`. A model may read the words
into ordinary typed facts, but those facts are a second source, `interpretation`, each linked to the
exact note version it came from, and each lands as a draft. Matt endorses or rejects a draft by a new
version; nothing is deleted. In Matt's words: "the words should be stored as a note and the
derivative facts or inferences should be linked back to the words as a source. We can have them on
stream as draft or preliminary and I can have a process where they are more formally endorsed by me
and the record superseded."

## The tension

Two pulls. Capture is where the value is: three of the four stories in
`docs/compelling-reason-for-Anam.md` are writes, and a record that only a device can write misses the
garden, the heat and the Hydralyte. Against that, a model that turns a sentence into a
`health.blood-donation` is making an interpretation, and Anam's doctrine is that a facet holds what its
source said, verbatim. Storing only the words keeps the doctrine and loses the structure; storing only
the model's facts gains structure and attributes the model's reading to the person.

Keeping both, as two sources, settles it. The words are the person's statement and are never
reinterpreted in place. The facts are the model's, and say so in their source, their link back and
their standing. A reader can always tell what Matt said from what was concluded from it.

## The rule

- **Words first, at once, and once.** The note is saved before anything is made of it, with no
  confirmation step. Its id is the moment and the words hashed, so a resend is a no-op; a correction
  names the id and is a new version (card 157, built).
- **A note may name no facet.** An account can come before the observation it describes: on
  11 October the heart rate alerts reached Anam after Matt had described the afternoon.
  `note.annotation` v3 made `about` optional, and a later version of the note names the facets.
- **Interpretations are their own source.** `interpretation` (name to confirm), with its own token
  whose role lists the types it may write: self-report types first, never a device's measurement
  type, so the watch's observation stays the watch's (card 89 holds the roles).
- **Standing is a value on each version.** `draft | endorsed | rejected`, after FHIR
  Observation.status. Endorsing writes a new version under the same source with `endorsed_by`,
  values corrected in the same step; rejecting writes `rejected`. Supersession stays inside one
  source's identity, which is why the endorsed version is not written under the person's source.
- **Drafts are shown and marked.** A projection or a standing filter drops them for a read shown to
  someone else.
- **Vague times keep their vagueness.** "Yesterday" is the local day as an interval; "earlier" is an
  interval bounded by what is known, with the zone and moment it was resolved against.
- **Correcting the words re-reads them.** Derived facts get new versions, back to draft, unless an
  endorsed fact's values come out the same.

## Rejected

- A records file written by the conversation and then scanned (`docs/API.md` §6 as first drafted).
  Overtaken once importers posted envelopes: a conversation is an importer like any other.
- Endorsement as a separate fact pointing at the draft's hash. Purer, but a correction then needs two
  facts and every read joins them; standing on the version gives the same history with one fact.
- Hiding drafts by default. Safer for sharing, but the assistant could not use what Matt said until he
  endorsed it, which defeats saying it.

Slices: cards 157 (built) to 161.
