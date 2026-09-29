# Journal

The record of Anam's creation — the journey, the decisions, and the *why* behind them.
Obsidian-flavored markdown; open this folder as a vault to navigate the links.

Two kinds of note:

- **Journal entries** (`YYYY-MM-DD-<slug>.md`, `type: journal`) — the chronological narrative of a
  session: what we explored, what we decided, where we are. Read these for the story.
- **Atomic notes** (`notes/<slug>.md`, `type: decision` / `type: note`) — one idea or decision per
  file, evergreen, capturing the tension it resolves, the alternatives rejected, and the rationale.
  Read these for the theory. Linked from the journal entries with `[[wikilinks]]`.

## Conventions

- **Atomic** — one decision per note; if a note needs "and", it's probably two notes.
- **Capture the tension** — a good note says what pulled the other way and why this side won.
- **Link liberally** — `[[the-inversion]]`; a link to a note that doesn't exist yet marks one worth
  writing.
- **Evergreen** — write for a reader (or a future us) who wasn't in the room. Carry the living theory,
  not just the surface decision (Naur).

## Index

- Journal entries
  - [[2026-07-10-genesis]] — the first session: from "my own Anam" to a committed spec.
  - [[2026-07-10-step-one-domain-types]] — building `facet`/`clock`/`config`, and a trap in the stdlib.
  - [[2026-07-10-r2-the-store-and-five-corrections]] — R2 built; five corrections, five different finders.
  - [[2026-07-10-r3-the-gate-and-an-inspector]] — R3 complete; the gate's choreography live, plus `anam inspect`.
  - [[2026-07-10-r4-the-fold-and-the-timeline]] — R4: the fold, the timeline, and the equality oracle.
  - [[2026-07-10-r5-the-big-import]] — R5: the first real source, and the big import.
  - [[2026-07-10-the-live-inspector]] — the inspector: capture→compose, drawn from the ledger.
  - [[2026-07-10-r6-changes-since]] — R6: `changes_since`, and the derived tier earns its first reader.
  - [[2026-07-11-bp-csv-arrives]] — the BP CSV arrives; identity isn't what the tests assumed. Plus safepath.
  - [[2026-09-29-the-health-pipeline-runs-end-to-end]] — three ETLs and none of them live; the fix was on unraid, not in Anam. Daily min and max, back to 2024.
  - [[2026-09-30-ingest-runs-on-a-clock]] — hourly, in the ThinkPad gateway. The gateway had never announced a failure, and a job in it had been stalled for three days.
  - [[2026-09-28-the-ledger-moves-to-sqlite]] — the ledger leaves DynamoDB for a SQLite file of its own; append-only is held by the database again.
  - [[2026-07-25-r7-projection-and-receipt]] — R7: the projection publishes, the receipt proves, the retry loop disappears; plus the viewer that switches between projections and winds the clock back.
- Decision notes
  - [[the-inversion]] · [[person-substrate-grain]] · [[mint-unit-whole-timeline]]
  - [[facet-types-dotted-namespaced]] · [[facet-definition-format]]
  - [[verbatim-payload-metadata-separation]] · [[guiding-lens-hickey-armstrong]]
  - [[fixed-width-timestamps-lexical-order]] · [[canonicalisation-fixes-serialisation-not-values]]
  - [[invariants-as-structural-tests]]
  - [[partition-key-has-no-default]] · [[config-explicit-fails-discovered-defaults]]
  - [[brooks-below-the-gate-beck-above-it]] · [[substrate-is-accidental]]
  - [[invariants-enforced-by-the-substrate]]
  - [[anam-owns-the-continuity]] · [[ledger-and-canonical-two-surfaces]]
  - [[span-is-the-mint-unit-not-the-read-filter]] · [[state-is-checkpoint-plus-replay]]
  - [[source-ref-is-identity-not-content]]
  - [[tombstone-is-a-record-event]] · [[published-is-a-receipt-not-a-record-event]]
  - [[sqlite-is-the-operational-store]] · [[identity-first-keys-and-no-indexes]]
  - [[checkpoint-is-a-replay-floor]] · [[doctrine-aphorisms-outlive-their-reasoning]]
  - [[identity-is-definition-owned]] · [[as-of-reads-fetch-by-version]]
  - [[one-decision-procedure-per-decision]]
  - [[ledger-is-a-sqlite-file-of-its-own]]
- Retracted
  - [[mint-unit-whole-timeline]] — superseded by [[span-is-the-mint-unit-not-the-read-filter]]
- Reference notes
  - [[reference-implementation-mint-cli]] · [[source-data-locations]]
