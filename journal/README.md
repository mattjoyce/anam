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
  - [[2026-10-06-r8-the-mint]] — R8: `anam checkpoint` mints one value per bucket; dirtiness is the value's hash, not a watermark, so no second index.
  - [[2026-10-06-what-meds-was-i-on]] — the self-log, documents as references, and the snapshot read; an interval is a series of point facts, and probable doses are not stored.
  - [[2026-10-06-the-inbox-and-the-first-documents]] — the records worker, the ACL parser, the runbook; 194 facts from the first documents, and the record now starts in 2023.
  - [[2026-10-07-series-snapshots-and-the-rest-of-the-board]] — nine cards to done in eleven commits: the series key, snapshots, reference payloads and the rest of the board.
  - [[2026-10-07-evening-the-rebuild-and-the-library]] — the blood pressure medicine named, the ledger rebuilt from source with a zone on every Definition, and the documents moved in beside it as a library.
  - [[2026-10-07-night-the-heart-rate-alarm-and-the-timeline]] — Garmin's heart rate alarm found in the raw FIT files, 301 times since January 2025, and a timeline rebuilt so an alarm looks like one.
  - [[2026-10-08-the-daemon-and-the-importers-proposal]] — `anam daemon` owns the stores behind a token and serves the CLI; Withings says how firmly a reading is tied to Matt; late that night the importers proposal moves the sources out of Anam.
  - [[2026-10-09-the-record-moves-and-reads-start-from-checkpoints]] — reads compose from checkpoints and buckets split by bytes along the calendar (980 ms to 62 ms); the record moves to unraid; nightly backups, closed JSON Schema, the importer door, a clean-room DreamStation decoder.
  - [[2026-10-10-the-importers-take-the-sources]] — Withings and the blood pressure exports arrive only through importers, DreamStation goes live with 1,317 nights, one owner per Definition, retraction by complete resend kept report-only, receipted reads over the API.
  - [[2026-10-11-health-db-retires-and-the-importers-take-over]] — eleven blue-team cards before breakfast; health-db retracted whole (5,818), Garmin and Concept2 live through importers sharing a bundled type, a read gets a size bound.
  - [[2026-10-12-the-phone-the-page-and-the-fields]] — OwnTracks positions as `location.track` with no map tiles; the timeline page from 1.84 MB to 100 KB; a read names its `fields` and gets a frame; the extractors leave for anam-importers.
  - [[2026-10-13-ten-cards-and-a-gate-misread]] — ten cards while Matt was away: checkpoints made visible, projections registered like Definitions, the gate refuses a type change, two browser holes closed, and card 108 merged on a misread gate.
  - [[2026-10-14-anam-reads-no-source-itself]] — records and documents move to their importers (2,901 resent, all no-op), the library names a document by sha256, every opinionated default gets a config key, queue wait spends the deadline, and a rebuild drill matches 2,939 facets but cannot bring back anam-time.
  - [[2026-10-15-words-first-then-what-a-model-makes-of-them]] — the sorting reviewed in four questions, the journal and its decision notes caught up, card 93 designed by Matt (words as a note, a model's reading as endorsed drafts), and card 157 lets an assistant save his words; card 89 gives every token a role.
  - [[2026-10-17-the-repos-go-public-the-places-i-stop-and-what-i-was-on-in-june]] — anam and anam-mcp go public; `location.stay` names the places Matt stops, a sibling facet from source `osm`, live with five stays; the supplements hand-written as `med.*` and the three guesses that make card 166's `handling` block; a snapshot projection took its snapshot at the head, not at `--at`.
  - [[2026-10-18-a-facet-with-eleven-versions]] — one sleep facet, two bugs below Anam: the importer stamped Sydney wall clock with `Z` since the container's zone was set, and GarminDB's analyze guard counted tomorrow's row (a Date key against datetime bind strings), rewriting the day every hour; both fixed, the churn's proof pending.
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
  - [[hard-and-soft-checkpoints]]
  - [[sources-reach-anam-only-through-importers]] · [[buckets-split-on-cost-and-cleave-on-the-calendar]]
  - [[one-owner-per-definition]] · [[retraction-by-resend-reports-and-never-tombstones-itself]]
  - [[retiring-a-source-is-a-separate-verb-from-reconciling-it]]
  - [[a-read-names-its-fields-and-gets-back-a-frame]] · [[a-consumer-selects-a-projection-the-operator-registers-it]]
  - [[the-type-is-not-in-the-identity-so-the-gate-refuses-a-type-change]]
  - [[the-library-names-a-document-by-its-sha256]] · [[a-hard-coded-value-is-a-default-an-invariant-or-presentation]]
  - [[queue-wait-spends-the-callers-deadline-up-to-acceptance]]
  - [[a-persons-words-are-a-note-and-a-models-reading-is-a-draft]]
  - [[a-token-names-a-principal-and-a-route-names-its-roles]]
  - [[an-external-reading-of-a-fact-is-a-sibling-facet-from-its-own-source]] · [[handling-is-where-to-look-not-a-vocabulary-anam-enforces]]
- Open decisions
  - [[a-rebuild-from-source-restores-the-facts-not-anams-knowledge]] — the drill's finding; whether the loss is acceptable is Matt's call (`docs/IMPORTERS.md` §6).
- Retracted
  - [[mint-unit-whole-timeline]] — superseded by [[span-is-the-mint-unit-not-the-read-filter]]
- Reference notes
  - [[reference-implementation-mint-cli]] · [[source-data-locations]]
