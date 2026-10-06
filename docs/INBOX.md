# Anam — the inbox: from a dropped document to facts in the ledger

How a file that Matt drops on the share becomes facets, and what each step promises. The design
is `SELF-LOG.md`; this is the runbook. Written 2026-10-06, the night the first documents went in.

## The folders

```
/mnt/Projects/anam/private/            Windows: \\192.168.20.4\Projects\anam\private\
├── <documents>                        dropped here; pending until processed
├── records/                           the SOURCE Anam ingests: one YAML per document
├── processed/                         documents whose facts are in the ledger
└── README-processing.md               status, for Matt, kept current by whoever processes
```

The clone of this repo on the share ignores `private/`. Nothing in it is ever committed.

## Records: the one shape every document becomes

`internal/capture/records` reads every `*.yaml` and `*.json` under `records/`:

```yaml
document:  {name: 2026-01-30_pathology_lipid_studies.pdf, sha256: a2716a1e…}
extracted: {method: scripts/extract-acl-pathology.py, version: 1}
records:
  - type: lab.result                   # a bundled Definition with identity_key ["/id"]
    id: acl:39986323/total-chol        # identity, chosen once, deterministic from the content
    collected: 2026-01-30 08:53        # fact-time, per the Definition's pointer and layout
    ...                                # the fact, as printed
```

- The payload is the record minus `type`. `document` and `extracted` are the file's provenance and
  **never enter the payload**: one measurement quoted by two reports is one fact, and the document
  would make the two fight. Which document said what is a grep over `records/`.
- Scalars reach the payload as typed: `20240916` is text, `2026-01-30 08:53` is text, `5.7` is a number.
- Source is `records`, so `source_ref` is `records:<id>` and the gate recomposes it from the
  Definition's identity_key. Every records type has `identity_key: ["/id"]`, except blood pressure,
  whose Definition composes `date_excel:reading`; a diary record's id is exactly that.
- One id in two files in one scan is refused. That is the guard against re-admitting a draw from
  a later report's history column when the draw has a report of its own.
- A re-run is a no-op; an edited record supersedes; a deleted record retracts nothing (card 17).

Types today: `lab.result`, `lab.panel`, `condition.noted`, `donation.blood`, `health.weight`,
`med.missed-dose`, `med.started`, `med.dose-changed`, `med.stopped`, `med.compliance-estimate`,
`illness.started`, `illness.ended`,
`illness.hangover`, `travel.departed`, `travel.arrived`, `travel.returned`, `visit.clinic`, `gym.visit`,
and `health.blood-pressure` for diary readings. Each is a manifest in `internal/def/manifests/`,
lenient, Sensitive (visits, gym and travel Confidential). Illness rows carry an `episode` and travel
rows a `trip`; those are the series keys, so `illness.ended` and `travel.returned` close what
`illness.started` and `travel.departed` opened, and `anam timeline --at` shows what was open on a day.

The three `med.*` regimen types are a **series** (`docs/SELF-LOG.md` §2): every row carries `drug` and
`course`, and a restart after a stop is a new course number. `anam timeline --at <date>` answers "what
was I on"; `anam serve meds` publishes the current regimen with a receipt.

```yaml
  - type: med.started
    id: self-log:med/ramipril/1/started
    date: 2024-09-16
    drug: ramipril
    course: 1
    dose: 5 mg daily
  - type: med.stopped
    id: self-log:med/ramipril/1/stopped
    date: 2025-02-01
    drug: ramipril
    course: 1
```

## Extractors

**ACL pathology PDFs** (Australian Clinical Labs, PDFsharp e-results) are deterministic:

```sh
python3 -I scripts/extract-acl-pathology.py --out-dir /mnt/Projects/anam/private/records \
  --history-labs 81225920 --known-times 81225920=07:56,22200789=07:46 <report.pdf>...
```

It runs `pdftotext -layout`, reads the header (Lab REF, REFERRED, COLLECTED, REPORTED), the
panel and specimen, the date/time/lab-number columns, each analyte row (values by column, `*`
flags, range, unit), and the lab's comment for the report's own draw. It emits `lab.result` per
analyte per draw and one `lab.panel` for the report's draw (`acl:<lab>/panel/<slug>`).

- **Which draws.** The report's own, plus the lab numbers in `--history-labs`. History columns are
  the lab restating an earlier draw; emit one only when no report of its own exists (the 2023 draw,
  lab 81225920, exists only as history). Never emit a history draw that has its own report.
- **Collection times.** History columns rarely print one; `--known-times` supplies them. One draw
  is one collection event, so the time is a fact about the lab number.
- **Ids.** `acl:<lab_number>/<analyte-slug>`; a non-blood specimen prefixes the slug
  (`urine-albumin`), so urine and serum albumin do not collide.

**Fitbit weight logs** (Google Takeout `Weight Logs.xlsx`): `scripts/extract-fitbit-weight.py`,
stdlib only, one `health.weight` per log, id `fitbit:<ISO minute>` with an ordinal when two logs
share a minute. **Strong workouts** (the app's CSV, semicolon-separated): `scripts/extract-strong-csv.py`,
one `gym.visit` per workout keyed by its start time (the export's "Workout #" is renumbered per export),
every set kept verbatim under `sets`.

**Lab ids name the lab that did the work.** `acl:<lab_number>/<analyte>` for Australian Clinical Labs,
`healthscope:<lab_number>/<analyte>` for Healthscope Pathology. ACL absorbed Healthscope in 2015 and
its later reports quote Healthscope draws as history columns under the old lab numbers; the prefix
keeps a draw's identity with the lab that collected it, whichever report prints it.

**Everything else is read.** Interleaved text (a PDF's text with columns scrambled), scanned
images, notes compiled from email: Claude reads the document and writes the records file by hand,
with `extracted.method` saying so. Matt's review of the YAML is the endorsement; the architecture's
`proposed` status (§11, R9) is not built, so the method field is the honest mark. A better reading
later supersedes under the same ids.

Scanned PDFs: `pdftoppm -r 90 -png` the whole file, read each page image; re-render at 160 dpi
when a digit is in doubt. A history row that only a later report prints (a lab that dropped a row from
its own report, or a draw whose report is lost) is emitted from that later report, with a
`source_note` saying so, under the original draw's lab number.

## Processing a drop

1. `sha256sum` the file. If a records file with that sha exists, the document is a duplicate: move
   it to `processed/` and stop.
2. ACL PDF: run the extractor into `records/`. Anything else: read it, write the records file.
3. Validate before touching the live ledger: build, `anam init` against a scratch ledger via
   `ANAM_LEDGER_PATH`, `ANAM_DB_PATH`, `ANAM_ARTIFACT_DIR`, `anam ingest --imports <scratch>`. The
   gate is the validator; a records file that fails here fails identically live.
4. `ANAM_PERSON_ID=matt ~/go/bin/anam ingest records` (back up `ledger.db` and `tracking.db` beside
   the data first, by convention `*.pre-<event>-<date>`). The hourly gateway ingest also picks up
   `records/`, since it is a declared import in `~/.config/anam/imports.yaml`.
5. `anam checkpoint` re-mints the buckets the new facts touched.
6. Move the document to `processed/`; update `README-processing.md`.

## What the first night taught

- A report's history columns are the only source for a draw whose report is lost (2023), and a
  trap for a draw whose report exists (2024). The dup-id refusal at scan time is what makes the
  policy safe to get wrong.
- Analyte names carry digits (Free T4); only the column gap separates a name from a value.
- Panel ids need their own segment, or the glucose panel collides with the glucose result.
- One report printed an ECG reported date of 10/03/2024 beside five panels saying 10/09/2024. It is
  recorded as September and the printed text is kept in the panel record. Reading is fallible, and
  the record says where.
- The live diary readings from Sep–Oct 2024 were not in the bp-csv export (which starts May 2025),
  so the diary PDF added a year of blood pressure, not duplicates. Check the ledger before assuming.
- A scanned text extraction can be wrong where the scan is right: blood_test1.txt read the ECG date as
  10/03/2024, the scanned original says 10/09/2024. The hand-read record was corrected and the record
  says which page settled it.
- Two children's results were in the 2016 scan. Other people's facts are not admitted to this record.
