# Anam — the self-log, documents, and the snapshot read

Design for the next sources: facts you tell Anam yourself, documents and images, and the read
that answers "what medication was I on, on that day". Written 2026-10-06 from a design conversation
the night the first checkpoint was minted; the journal entry is `2026-10-06-what-meds-was-i-on`.
Where this document is silent, `ARCHITECTURE.md` governs. Nothing here touches the error kernel.

---

## 1. The question that drove it

*What meds was I on, as of a date?*

Anam already answers two of the three clocks in that sentence. Fact-time (`--from/--to`) asks when
things happened; anam-time (`--as-of`) asks when Anam found out; both compose. What it cannot do is
the third thing the question needs, which `ARCHITECTURE.md` §4 names and marks not built: a
**last-known-value read**, one facet per series, the latest at or before a fact-time. Medications are
the first data where that is the only useful answer.

## 2. The pattern: point facts, series, snapshot

**A facet has one fact-time, and it is the only time a read may use.** Fact-time is metadata; the
payload is opaque (§3). So an interval in the world is never one facet dated at its start. A regimen
that runs March to June, stored as one fact dated March, is invisible to a May query.

**An interval is a series of point facts.** Started, changed, stopped, each with its own fact-time,
each immutable, each a row in the source. The kind of change goes in the **facet type**, so it is
metadata and no read opens a payload: `med.started`, `med.dose-changed`, `med.stopped`.

**The series key says which facts are the same thing over time.** Identity is too fine (every change
has its own id) and type is too coarse. A Definition declares a series **family** and the pointers
that name one series within it, the drug and the course, and the gate stamps `<family>:<values>` into
metadata at capture, as it stamps classification today. A Definition also declares whether its type
**closes** a series (`med.stopped`), so "a stop ends the regimen" is data in the Definition, not a rule
in the read. Built 2026-10-07 (card 15; the design pass is `journal/notes/series-key.md`):

```json
"series": {"name": "med.regimen", "key": ["/drug", "/course"], "closes": false}
```

Every type in a family declares the same key, and the registry refuses a family keyed two ways at load.
`fold.Snapshot` is the read; the registry answers which types close.

**The snapshot read.** For a date D: take every facet with fact-time at or before D, group by series,
keep the latest in each, drop the series whose latest is a closing type. What remains is what was true
at D for every series Anam knows. Types without a series key are left alone. With `--as-of T` first,
the same read is bitemporal: what Anam believed on T was true at D.

**Checkpoints do not look backward, and the snapshot does not need them to.** A checkpoint is every
live facet in one bucket (§7). The snapshot composes the **prefix** of hard checkpoints up to D's
bucket, the same union the timeline would do, and folds per series over that. A drug started in 2024
and never stopped is carried through every month since, because the prefix includes 2024. A
carry-in per bucket boundary (an opening balance) would read faster and would make every later
bucket depend on every earlier one, which is the cross-bucket dependency fact-time bucketing was
chosen to avoid. Measure the prefix read first; the carry-in is the known next move if it is ever slow.

**The snapshot is soft.** It is computed, shown, dropped. A snapshot you want to cite is served
through a snapshot projection, and the receipt pins it (`GLOSSARY.md`, *Checkpoint, soft*).

## 3. The self-log: a source you keep by hand

A plain file you edit, one row per event, read by a capture worker like any other source. It is a
**source**, outside the gate, so Anam owns none of it and re-reading it is idempotent.

- **Identity is a row id you assign**, stable for the row's life, never its content and never its
  line number. `source_ref = self-log:<row_id>`. Editing a row's fields supersedes the facet;
  deleting a row does not tombstone it by itself: `anam retract <source>` reports the rows the
  source has dropped and tombstones them only with `--yes` (card 17, 2026-10-07), so a wrong row is
  corrected in place, and a row that should never have been is retracted on purpose.
- **Fact-time is the row's date**, the day the thing happened, as you record it. A row added late is
  a backfill and re-opens its bucket, which is the system working.
- **Validation is closed** for every self-log type (card 52). A row carries as much or as little as
  you know of the fields its Definition declares: "Went to the gym" is a row, and "Went to the gym,
  45 minutes, legs" is the same row with more. A field the Definition does not declare is refused,
  so a new kind of detail is a line in the manifest first.
- **Classification** is Sensitive for everything medical; travel and gym can be Confidential.
- Format: **records files**, one YAML per source document, read by `internal/capture/records`
  (decided and built 2026-10-06; `INBOX.md` is the runbook). A hand-kept log is one records file
  whose document is the log itself.

## 4. The types

| Namespace | Types | Shape | Series |
|---|---|---|---|
| `med` | `med.started`, `med.dose-changed`, `med.stopped` | series of changes | drug + course; `med.stopped` closes |
| `med` | `med.compliance-estimate` | point: a statement you make on a date about a period | none |
| `lab` | `lab.result` | point per analyte, fact-time = draw date | analyte |
| `doc` | `doc.reference` | the reference facet for a file or image (§5) | none |
| `donation` | `donation.blood` | point | none |
| `illness` | `illness.started`, `illness.ended`, `illness.hangover` | series for an illness; hangover is a point | `illness.episode` keyed by `/episode`; `illness.ended` closes |
| `travel` | `travel.departed`, `travel.arrived`, `travel.returned` | series per trip; arrived is a leg within it | `travel.trip` keyed by `/trip`; `travel.returned` closes |
| `travel` | `travel.flight` | point per flight leg: `from` and `to` as IATA codes, dated where it left, `date` carrying the departure airport's offset (`2018-10-10 +01:00`) | none; it carries `/trip` to group with its trip but stays outside the series |
| `gym` | `gym.visit` | point, generic or detailed | none |

Hangover is a point fact under `illness` because that is what it is; one row, one day.
A flight is evidence of a movement, not a state of the trip. Inside `travel.trip` a flight
dated in Singapore would sort after a return dated the same day in Sydney and hold the trip open,
so the trip's state is departed, arrived and returned, and flights stand beside it (card 35). Detailed gym
data that a device already records (Garmin activities) stays with its device source; `gym.visit` is
your word that you went, and the two can be correlated later, never merged.

## 5. Documents and images

`ARCHITECTURE.md` §3 reserved a payload kind, `reference`: a facet whose payload is a native reference
to a binary in its source, with `media_type` and `content_hash` in the descriptor. Nothing has needed
it until now. Rules:

- **The bytes stay in the source.** A folder of scans is a source. Anam holds the path, the hash, the
  media type, never the bytes. Anam owns no facts, and the ledger payload is for structured bodies.
- **Identity is the path, not the hash.** `source_ref` is identity, not content (§6a). A renamed
  file is a new facet and the old one is tombstoned, which is honest.
- **Fact-time comes from outside the bytes**: EXIF, the document's own date, or the self-log row
  that points at it. If fact-time lived only inside the image, every read would open every image.
  Built as a sidecar, `<file>.meta.yaml` with `date: YYYY-MM-DD`, beside the file; a file without
  one is refused and the message names the sidecar to write (`INBOX.md`).

**Extracted data is a second facet, derived.** OCR or a model reading a blood test is fallible, so
what it produces re-enters through the same gate as a derived facet (§11): admitted `proposed`, its
provenance naming the method and model version and the `doc.reference` it was read from. You
endorse it. A better extractor later supersedes it, same identity, new content. Fact-time is the
draw date in the document, not the day it was scanned. Text files that a plain parser can read need
no model, and their results are ordinary admitted facts.

## 6. Prescriptions and compliance

A prescription document is a `doc.reference`. The regimen it describes is `med.started` rows in the
self-log, each pointing at the document. The document is evidence; the row is the fact.

Your estimate, "95% compliant on this one", is **also a fact**: you said it, on a date, about a
period. `med.compliance-estimate` carries the number, the period, and the course it is about. Anam
stores it and does not judge it. A later estimate is a new facet, not a correction.

**Probable doses are not stored.** Seeding a `took-it` facet per day at p = 0.95 would store 365
facets that carry no information beyond the two they derive from. An expected-dose schedule is a
soft derivation from the regimen and the estimate: compute it when an agent needs it, drop it. Two
things do earn a stored dose event, because they happened and have a source: an observation (a pill
tracker, a photo of the blister pack) and a specific assertion ("I skipped Tuesday"). If a schedule
ever has to be cited, §11 allows it as `proposed` derived facets with a confidence field, isolated
by status from world facts. Do that on demand, not by default.

## 7. Build order

1. ✅ **Series key and closing type** in the Definition, stamped at capture (card 15, 2026-10-07).
2. ✅ **The self-log source**: format, row identity, capture worker (card 22, 2026-10-06).
3. ✅ **Medication types and the snapshot read**, with a snapshot projection and `--at` (card 23,
   2026-10-07): `med.started` / `med.dose-changed` / `med.stopped`, the `meds` template (shape
   `snapshot`), `timeline --at`. The inspector's projection page shows the same verdicts.
4. ✅ **Life events** as self-log types: donation, illness, hangover, travel, gym (card 27, 2026-10-07;
   illness and travel are series, closed by `illness.ended` and `travel.returned`).
5. ✅ **Reference payload kind** and a folder source (card 24, 2026-10-07): `doc.reference`, the
   `documents` worker over a folder with `<file>.meta.yaml` sidecars carrying the date.
6. **Lab results** from text files first, then images via a `proposed` extractor (card 25).
7. ✅ **Compliance estimates** (card 26, 2026-10-07): `med.compliance-estimate`, the estimate bounded
   to (0, 1] at the gate; `internal/regimen.Expected` is the soft schedule, and a test holds that the
   watermark does not move when it is computed.

## 8. Tests that define done

- The snapshot at D from composed hard checkpoints equals the snapshot from a full fold, and both
  equal a hand-computed answer over a seeded log with a start, a dose change, a stop, and a restart
  as a new course.
- One bitemporal case: the snapshot at D as Anam knew it before the stop row arrived.
- A self-log row edited in place supersedes; re-reading the file admits nothing new.
- A `doc.reference` whose file is renamed yields a new facet, never a changed one.

## 9. Open questions

- ~~Whether a series key is one declaration or two (group and course).~~ One declaration, with a
  family name and an ordered key; the course is a key element, so a restart is a new series (2026-10-07).
- ~~Whether `travel.arrived` closes a trip only when the destination is home, which reads a payload,
  or whether the trip is closed by a distinct `travel.returned` type.~~ The type: `travel.returned`
  closes, `travel.arrived` is a leg within the trip (2026-10-07).
- Where the self-log file lives and how it reaches the ThinkPad gateway's hourly ingest.
