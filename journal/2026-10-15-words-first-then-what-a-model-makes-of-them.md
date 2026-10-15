---
type: journal
date: 2026-10-15
tags: [anam, journal, conversation, notes, config, journal-backfill, auth, rbac]
---

# 15 October: words first, then what a model makes of them

The morning after the overnight list. Matt reviewed the sorting from cards 151 to 153 in four
questions, the journal caught up a week it had missed, and card 93, which had sat in todo since the
API review, went from "decide how a conversation writes" to a design Matt shaped himself and a first
slice merged. By the end, an assistant could save Matt's words as a note, waiting only on a token
being registered on unraid.

## The journal catches up

The journal stopped at the night of 7 October, and seven days of work since had gone only into
commits, cards and the private ZK vault. Seven entries went in, one a day, drawn from the commits
across anam, anam-importers and anam-mcp and the week's ZK notes. The index had also missed the three
7 October entries.

Matt then asked whether the entries carried decisions and rationale. They did, inline, but the
journal's second layer had stalled: the last decision note in `journal/notes/` was the Open mHealth
one of 8 October. Twelve went in, one per decision, each with its tension: from
[[sources-reach-anam-only-through-importers]] to
[[queue-wait-spends-the-callers-deadline-up-to-acceptance]], and
[[a-rebuild-from-source-restores-the-facts-not-anams-knowledge]] filed under a new "Open decisions"
heading, since whether that loss is acceptable is Matt's call. Two needed reconciling as they were
written: card 125's tier (the card said provisioned, ARCHITECTURE §8 says authored, and the note
follows the code), and the defaults note, which refines `config-explicit-fails-discovered-defaults`
rather than contradicting it.

## The sorting, reviewed

Card 151's list ran to about 120 values across the three repos, and most of it was not in question.
Four calls were judgement, and went to Matt as questions with a recommendation each: whether
presentation and store tuning are a third kind that stays constant (yes); whether "no test hooks",
the agent's suggestion, holds (confirmed); whether any invariant should be a key instead, the file
modes, the importers' batch size or records' restated identity key (none); and what to do with
`ingest_timeout`, a key named for something that no longer exists (rename it, reading the old name
as an alias, card 156). The code already matched every answer. The lesson for the asking: of 120
lines, four needed a person.

## Card 93: the shape of a conversational write

The card had been overtaken in part already: its records-file route predates importers, and a
conversation is just one more importer posting envelopes. What was left were the questions that make
it trustworthy, and Matt answered the biggest one before it was asked. Matt wants a model to interpret
the words, but the words stored as a note and every inferred fact linked back to them as its source,
landing as a draft Matt later endorses, superseding it.

That fits the doctrine better than either half alone. A facet holds what its source said, so the words
are the note's payload, verbatim. A model's reading is a different source, `interpretation`, so the
record never says Matt said what a model concluded. Two rounds of questions filled in the rest: standing as a value on each version (`draft | endorsed |
rejected`, after FHIR's Observation.status) rather than a separate endorsement fact; drafts shown and
marked rather than hidden; endorsement in conversation, from a review list, and on an inspector page;
vague times as an interval with their anchor; re-reading when the words change; and the interpreter
limited to self-report types, which Matt pointed out belong in the token's role, card 89, so widening
them is configuration. The decision is
[[a-persons-words-are-a-note-and-a-models-reading-is-a-draft]]; the slices are cards 157 to 161.

## Card 157: the words, saved

The first slice is the words alone. Two changes, two repos.

In core, `note.annotation` v3 makes `about` optional. An account can come before what it describes:
on 11 October Matt described the afternoon before the heart rate alerts reached Anam. Checking the
version history for the bump turned up a gap. Card 136 had moved the note to v2 without copying v1
into `manifests/history`, so a rebuilt operational store would have lost the words Matt's first note
was admitted under. v1 and v2 went in with v3, and the history test now holds the note to all three.

In anam-mcp, `save_note`, offered only when `ANAM_MCP_NOTE_TOKEN_FILE` is set, so a deployment that
names no write token stays read-only. It reads the note Definition's fact-time layout and zone from the
daemon instead of assuming them, saves at the present moment, and ids the note by the moment and the
words hashed, so the same words sent twice are one note. A correction passes the id and time and
becomes a new version.

## Hickey × Armstrong

**Hickey.** The temptation was one type and one source for everything said in a conversation. Two
sources keep two different things apart: what a person said, which is fixed the moment it is said, and
what was made of it, which can be wrong and corrected. Standing as a field on each version keeps the
Datomic shape: each version is a value, and "is this endorsed" is read from the latest one, never
updated in place.

**Armstrong.** The review found the failure the tests had not. The note's id hashes the minute, so a
save whose answer is lost, retried by the model a minute later, would land as a second note. A failed
save now names the id and time to retry with. The token never left the ThinkPad: it was made there,
readable only by Matt, and only its hash went to unraid, which is all the daemon keeps.

## Live

ThinkPad.unraid deployed v0.323-60f1988 over v0.314, backing up first (W 33188 before and after), and
registered the `conversation` importer by the token's hash. No Claude client had the MCP server
registered yet, so the first save drove its stdio by hand: `initialize`, `tools/list`, then
`save_note` twice. Matt's card 93 words were admitted as `conversation:8393535598127b22` at W 33200,
the second send was a no-op, and a timeline read shows one note under source `conversation`.

The save also showed a gap. Those words were said on 14 October and the note is dated 15 October
11:49, the moment of saving, because a new note always takes the present and a correction keeps the
note's time. Card 162 lets a save say when the words were said.

## Card 89: tokens carry roles

The note token made a gap concrete. The adapter read with the operator's token and saved with the
`conversation` importer's, because an importer could not read and the operator could not submit.
Card 89 had been reshaped on 11 October from "a token that can only read" into role-based tokens,
and it went in the same afternoon. Every token now names a principal with a role, `reader`,
`importer` or `operator`, and one route table names the roles each route admits
([[a-token-names-a-principal-and-a-route-names-its-roles]]). An importer reads as a reader does,
so one token can carry a conversation. The operator's token is held as a hash like the others, and
the live importers file reads unchanged, with readers going under a new `principals` key.

A test sends each role's token to all 22 routes. Since the table is the only way a route is
registered, nothing escapes it. Importers changed in two places. They read where they were refused
(200, was 401), and the operator's routes now refuse them with 403 rather than 401.

Running the anam-importers suite against the branch gave one error. The same build of master gave
the same error: the records cut-over test still calls `anam ingest --imports`, which card 119
removed. Card 164 retires it. Merged as 9f9a317; deploying it is ThinkPad.unraid's.

**Hickey.** Three wrappers that each decided access their own way became one middleware and a
table. What stays braided is a name: `importers_file` now holds readers too.

**Armstrong.** A malformed file stops the daemon at start with the reason. An unknown token is
refused before any handler runs. No write path changed, so the gate and the ledger are untouched.

## Left open

- Cards 162, dating a note when the words were said, and the MCP server's registration in a Claude
  client, with the note token.
- Cards 158 to 161: the interpreter, endorsement, the inspector's review page (its first write to the
  record), and re-reading corrected words. The source's name, `interpretation`, is still to confirm.
- Card 141's unraid half, and the two rebuild questions in `docs/IMPORTERS.md` §6.
- Card 156, the `ingest_timeout` rename.
- Card 89 live: the deploy, a reader principal registered, and the adapter moved off the
  operator's token, onto a reader token or onto the conversation importer's token alone.
- Card 164, the stale records cut-over test in anam-importers.
