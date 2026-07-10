---
id: 202607102355
type: note
title: "The doctrine is written in aphorisms, and aphorisms outlive their reasoning"
created: 2026-07-10
status: evergreen
tags: [anam, note, doctrine, drift, method]
project: anam
related: ["[[tombstone-is-a-record-event]]", "[[sqlite-is-the-operational-store]]", "[[published-is-a-receipt-not-a-record-event]]", "[[the-inversion]]"]
---

# The doctrine is written in aphorisms, and aphorisms outlive their reasoning

`ARCHITECTURE.md` is written in memorable absolutes. They read as settled law, they propagate by
copy-paste into `AGENTS.md`, `MVP.md`, the glossary and config comments, and several arrived verbatim
from Arch, where they were true of a different system. An aphorism compresses a decision into a slogan
and discards the reasoning — so nothing signals when the premise changes.

One session found three false ones, none previously caught **because no code had yet depended on them**:

- *"a status-bearing artifact version carrying `tombstoned`"* — Arch's tombstone; contradicted by
  Anam's own type system since R0 ([[tombstone-is-a-record-event]]).
- *"Truth is immutable and lives in exactly one place… SQLite holds nothing that cannot be rebuilt"* —
  demoted a real store to a cache and put `def#` in the ledger by implication
  ([[sqlite-is-the-operational-store]]).
- *"`init` creates … append-only triggers"* — a survival from the all-SQLite phase, months after the
  substrate changed.

Also in this family: the glossary/§7/§8 three-way disagreement over whether `published` advances `W`
([[published-is-a-receipt-not-a-record-event]]) — each document quoting a different draft of the same
slogan.

Two working rules, both earned by pushback rather than foresight:

1. **Before building on a quotable sentence, check it against the code** — and if it came from Arch,
   against the ADR it came from. *Read the decision records, not the summary* was already this session's
   inheritance ([[the-inversion]] was found that way); this note extends it from *reading* prior art to
   *inheriting* it.
2. **The fix for an over-strong rule is not a better-worded rule.** The replacement universal drifts the
   same way. Replace it with a per-case fact — here, a per-table recovery tier. Facts age visibly;
   slogans age silently.
