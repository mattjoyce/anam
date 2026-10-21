# Anam — the inbox: from a dropped document to facts in the ledger

How a document dropped on the share becomes facets, and what each step promises. The design is
`SELF-LOG.md`; this is the runbook.

## The folders

```
/mnt/Projects/anam/private/            the INBOX (the path is the author's; any folder will do)
├── <documents>                        dropped here; pending until processed
└── README-processing.md               status, for the person, kept current by whoever processes

/mnt/Library/anam/                     the SOURCES, on the Library share, which the gateway reads
│                                      (/mnt/user/Library/anam, at /app/anam-library read-only)
├── records/                           one YAML per document, read by anam_records
└── documents/                         every processed primary document, read by anam_documents;
    ├── <document>                     each with a sidecar, each a record.document facet;
    └── <document>.meta.yaml           `date:` the document's own date

<the daemon's data directory>          the RECORD, and the backup unit
├── ledger.db                          the ledger
├── tracking.db                        the operational store
├── artifacts/                         checkpoints and receipts
└── documents/                         the LIBRARY: the bytes of every referenced document,
                                       written by Anam alone from what anam_documents posts
```

The two sources reach the record through their importers on the gateway, `anam_records` and
`anam_documents`. The sources sit on the Library share because the gateway cannot read
`anam/private`. Anam reads no folder itself.

The clone of this repo on the share ignores `private/`. Nothing in it is ever committed. The library
is the primary source: a records file is an extraction from a document that is in the library, with
the hash its `document.sha256` records; its `document.name` is the document's path under
`documents/`, its name and identity. The library itself names each file by its sha256.

## Records: the one shape every document becomes

`anam_records` reads every `*.yaml` and `*.json` under `records/`:

```yaml
document:  {name: 2026-01-30_pathology_lipid_studies.pdf, sha256: a2716a1e…}
extracted: {method: records/extract/acl_pathology.py, version: 1}
records:
  - type: health.lab-result                   # a bundled Definition with identity_key ["/id"]
    id: acl:39986323/total-chol        # identity, chosen once, deterministic from the content
    collected: 2026-01-30 08:53        # fact-time, per the Definition's pointer and layout
    ...                                # the fact, as printed
```

- The payload is the record minus `type`. `document` and `extracted` are the file's provenance and
  **never enter the payload**: one measurement quoted by two reports is one fact, and the document
  would make the two fight. Which document said what is a grep over `records/`.
- Scalars reach the payload as typed: `20240916` is text, `2026-01-30 08:53` is text, `5.7` is a number.
- Source is `records`, and the worker composes `source_ref` from the type's Definition exactly as
  the gate does (`facet.SourceRef`: the source, then each identity value length-prefixed). Every
  records type has `identity_key: ["/id"]`, so the ref is `records:<len>:<id>`; blood pressure is keyed
  on `date_excel` and `reading`, and a diary record's `id` is then just another field.
- One id in two files in one scan is refused. That is the guard against re-admitting a draw from
  a later report's history column when the draw has a report of its own.
- A re-run is a no-op; an edited record supersedes; a deleted record retracts nothing until you ask:
  `anam retract records` lists what the folder no longer holds, and `--yes` tombstones it. An empty or
  unreadable folder is refused, and more than a tenth of the source in one run needs `--max-fraction`.

Types today: `health.lab-result`, `health.lab-panel`, `health.condition`, `health.donation`, `health.weight`,
`health.medication.dose-missed`, `health.medication.started`, `health.medication.dose-changed`, `health.medication.stopped`, `health.medication.compliance`,
`health.illness.started`, `health.illness.ended`,
`place.trip.departed`, `place.trip.arrived`, `place.trip.returned`, `place.trip.flight`, `health.encounter`, `activity.gym-visit`,
`health.vision-prescription`, and `health.blood-pressure` for diary readings. Each is a manifest in
`internal/def/manifests/`, strict, Sensitive (visits, gym, travel and the lens prescription
Confidential). Each schema is closed: a records row may
carry only the fields its Definition declares, so a new field means a line in the manifest first. Illness rows carry an `episode` and travel
rows a `trip`; those are the series keys, so `health.illness.ended` and `place.trip.returned` close what
`health.illness.started` and `place.trip.departed` opened, and `anam timeline --at` shows what was open on a day.

The three regimen types, `health.medication.started`, `health.medication.dose-changed` and
`health.medication.stopped`, are a **series** (`docs/SELF-LOG.md` §2): every row carries `drug` and
`course`, and a restart after a stop is a new course number. `anam timeline --at <date>` answers "what
was I on"; `anam serve meds` publishes the current regimen with a receipt.

```yaml
  - type: health.medication.started
    id: self-log:med/ramipril/1/started
    date: 2024-09-16
    drug: ramipril
    course: 1
    dose: 5 mg daily
  - type: health.medication.stopped
    id: self-log:med/ramipril/1/stopped
    date: 2025-02-01
    drug: ramipril
    course: 1
```

## Documents as facts of their own

Every processed document is a facet: `record.document`, whose payload is the document's path in the
source folder, the sha256, the media type and size, and the date from a sidecar. `anam_documents`
reads the share's `documents/` folder, and `anam_submit` posts each file's bytes to
`POST /v1/documents` before its reference, so the **library** beside the ledger holds them and Anam
refuses a reference to bytes it does not hold. The library names each stored file by its sha256, so
a document rescanned under the same name is new bytes, and its facet supersedes the old one, whose
bytes stay. Every file in the source folder has a sidecar beside it,
`<file>.meta.yaml`:

```yaml
date: 2026-01-30          # the document's own date, not the day it was scanned
title: Lipid studies      # anything else worth saying stays in the payload too
```

A file with no sidecar stops the importer's run and names the sidecar to write, so write the
sidecar before the file goes in. Identity is the relative path, so a renamed file is a new facet and the old one
stands until `anam retract documents --yes` plans it from the importer's next whole resend; changed
bytes at the same path supersede. `.meta.yaml` files, dot-files and dot-folders are skipped. The
inspector links a reference facet to its bytes (`/document?sha256=…`), served from the library and
confined to it, under the document's own file name.

A second documents folder is a second `anam_documents` instance with its own `documents_dir`; the
folder is config and never enters a fact, and every path is relative to it.

There are two ways to make a document a fact, and the difference is who keeps the bytes. Use
`record.document` for a primary document handed to the library: the bytes are posted to Anam, the
library is their backup, and a receipt over the facet can always reach them. Use `record.link`
(from `anam_links` in anam-importers) for a file that belongs to another tool and stays
where it lives, a note in a vault, a photo in a camera roll, a screenshot: the facet records that a
file with this name, sha256, size and date was seen under a declared folder, and Anam takes no
bytes, follows no path and serves nothing from the folder. A `record.link` whose sha256 the
library does not hold is admitted, since the held-bytes refusal is keyed on `record.document`; the
inspector says the bytes are not held and offers no download. When the importer stops finding the
file it supersedes the facet with `unseen: true` and never retracts it, so the earlier version
stands as what was. A receipt over a `record.link` therefore promises the observation, that this
was recorded of the file at that time, and never the bytes.

## Extractors

The extractors live in `anam-importers` under `records/extract`. Anam holds no copies. Run them from that repo.

**ACL pathology PDFs** (Australian Clinical Labs, PDFsharp e-results) are deterministic:

```sh
python3 -I records/extract/acl_pathology.py --out-dir /mnt/Library/anam/records \
  --history-labs 81225920 --known-times 81225920=07:56,22200789=07:46 <report.pdf>...
```

It runs `pdftotext -layout`, reads the header (Lab REF, REFERRED, COLLECTED, REPORTED), the
panel and specimen, the date/time/lab-number columns, each analyte row (values by column, `*`
flags, range, unit), and the lab's comment for the report's own draw. It emits `health.lab-result` per
analyte per draw and one `health.lab-panel` for the report's draw (`acl:<lab>/panel/<slug>`).

- **Which draws.** The report's own, plus the lab numbers in `--history-labs`. History columns are
  the lab restating an earlier draw; emit one only when no report of its own exists (the 2023 draw,
  lab 81225920, exists only as history). Never emit a history draw that has its own report.
- **Collection times.** History columns rarely print one; `--known-times` supplies them. One draw
  is one collection event, so the time is a fact about the lab number.
- **Ids.** `acl:<lab_number>/<analyte-slug>`; a non-blood specimen prefixes the slug
  (`urine-albumin`), so urine and serum albumin do not collide. A panel's id takes its own
  `panel/` segment, or a panel named for an analyte (glucose) collides with that analyte's result.
- **Analyte names.** A name can contain digits (Free T4), so the extractor separates a name from its
  value by the column gap, never by the first digit.
- **A history column is a trap as well as a source.** It is the only record of a draw whose report
  is lost, and a duplicate of a draw whose report exists. The extractor's `--history-labs` list
  decides, and a wrong choice is caught at scan time, when one id appears in two files.

**Fitbit weight logs** (Google Takeout `Weight Logs.xlsx`): `records/extract/fitbit_weight.py`,
stdlib only, one `health.weight` per log, id `fitbit:<ISO minute>` with an ordinal when two logs
share a minute. **Strong workouts** (the app's CSV, semicolon-separated): `records/extract/strong_csv.py`,
one `activity.gym-visit` per workout keyed by its start time (the export's "Workout #" is renumbered per export),
every set kept verbatim under `sets`.

**Lab ids name the lab that did the work.** `acl:<lab_number>/<analyte>` for Australian Clinical Labs,
`healthscope:<lab_number>/<analyte>` for Healthscope Pathology. ACL absorbed Healthscope in 2015 and
its later reports quote Healthscope draws as history columns under the old lab numbers; the prefix
keeps a draw's identity with the lab that collected it, whichever report prints it.

**Everything else is read.** Interleaved text (a PDF's text with columns scrambled), scanned
images, notes compiled from email: Claude reads the document and writes the records file by hand,
with `extracted.method` saying so. The person's review of the YAML is the endorsement; the architecture's
`proposed` status (§11, R9) is not built, so the method field is the honest mark. A better reading
later supersedes under the same ids.

Scanned PDFs: `pdftoppm -r 90 -png` the whole file, read each page image; re-render at 160 dpi
when a digit is in doubt. When the text extraction and the scan disagree, the scan is the document:
correct the record and say which page settled it in `source_note`. A history row that only a later report prints (a lab that dropped a row from
its own report, or a draw whose report is lost) is emitted from that later report, with a
`source_note` saying so, under the original draw's lab number.

## Reading a document

- **Record what is printed, and say where it is doubtful.** A document can contradict itself (a
  header date that differs from the dates on its own panels). Record the date the rest of the
  document supports, and keep the printed text in the record's `source_note`. Reading is fallible,
  and the record says where.
- **Admit only the person's own facts.** A document that also carries someone else's results (a
  child's, in a family scan) contributes only the person's own rows.
- **Check the ledger before judging a range.** A new source can reach back before an existing
  export begins, so it adds readings and does not duplicate them. Look at what the ledger holds
  for those dates (`anam timeline --from … --to …`) first.

## Processing a drop

1. `sha256sum` the file. If a records file with that sha exists, the document is a duplicate: delete
   the drop and stop (the library already holds it).
2. ACL PDF: run the extractor into `records/`. Anything else: read it, write the records file. Its
   `document.name` is the path the document will have under `documents/`.
3. Validate before touching the live record: run `anam_records`' health check on the folder, or
   send it to the security lab daemon (`anam-lab`, synthetic data only, so a scratch copy of the
   one file). The gate is the validator; a records file that fails there fails identically live.
4. Move the document into `/mnt/Library/anam/documents/` and write its sidecar.
5. The importers send both on their next poll, the records file and the document's bytes and
   reference. The daemon mints after the batches.
6. Update `README-processing.md` on the share.
