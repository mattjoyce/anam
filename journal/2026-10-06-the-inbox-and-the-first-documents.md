---
type: journal
date: 2026-10-06
session: 12
title: "The inbox: 194 facts from the first documents"
tags: [anam, journal, inbox, records, lab, extraction, self-log]
---

# The inbox: 194 facts from the first documents

Third act of the same evening ([[2026-10-06-r8-the-mint]], [[2026-10-06-what-meds-was-i-on]]).
Matt made a `private/` folder on the share, another Claude went through his Gmail and dropped in
six pathology PDFs, a blood pressure diary, a zip of scans and a dozen markdown notes, and Matt
said: *you need to process them. This is your work. Get Anam into a state where we can create my
health record, bit by bit.*

## What was built

The **records worker**, `internal/capture/records`: every document becomes one YAML file of
records, each record a typed fact with an id Anam uses as identity. The payload is the record and
nothing else; the file's `document` and `extracted` are provenance and stay out of it, because a
lab prints last year's column beside this year's and one measurement must stay one fact however
many reports quote it. Scalars reach the payload as typed, so `20240916` is text and a time is a
time string, not what YAML guessed. One id in two files is refused at scan. Eight new Definitions,
all lenient, identity `/id`. Blood pressure diary readings reuse the bp-csv Definition by making
the id its identity composition, which the gate then verifies.

The **ACL parser**, `scripts/extract-acl-pathology.py`: `pdftotext -layout` keeps the columns, and
the result tables are regular enough to read exactly. Two bugs on the way: analyte names carry
digits (Free T4), so only a two-space gap separates a name from a value; and the glucose panel
collided with the glucose result until panels got their own id segment.

The **runbook**, `docs/INBOX.md`: folders, the records shape, the extractors, the six steps of a
drop, and the policy for history columns.

## What went in

194 facets at W=11575, validated first on a throwaway ledger through the real gate, then live:
144 lab results across three draws (Sep 2023 recovered from history columns, Sep 2024 read by hand
from interleaved text, Jan 2026 parsed), 12 panels with the lab's comments, 25 blood pressure
readings from Sep–Oct 2024 that the CSV export never had, three weights, one missed dose, two
conditions noted on a request form, three blood donation visits, three clinic visits, one row. The
record now runs from January 2023. Twelve buckets re-minted.

## Decisions worth keeping

- **Provenance is the file's, not the fact's.** The records file is kept forever beside the data;
  which document said what is a grep. Putting the document in the payload would have made two
  reports of one measurement supersede each other forever.
- **History columns are emitted once, from one report, by policy**, and the dup-id refusal is the
  net under the policy.
- **Reading is marked, not blocked.** `proposed` (R9) is not built. A hand-read records file says
  so in `extracted.method`, Matt can open it from Windows, and a better reading supersedes.
- **A later report's quoted value may differ from the original's.** If it does, supersession will
  show it; that is the system noticing, not a bug.

## Next

Card 28: the zip of scans, 26 image pages from 2013 to 2024, the oldest pathology there is. Then
the antihypertensive's name, which nothing in the mailbox gives.

## Later the same night: the scans, Fitbit and Strong

Matt kept dropping. The zip of scans turned out to be the oldest pathology there is, 26 image pages
from 2013 to 2024, read page by page at 90 dpi and re-rendered at 160 where a digit was in doubt.
Then a Fitbit weight log (1,597 weights, 2015 to 2024) and the Strong app's workout export (409
workouts, 4,818 sets, 2020 to 2026), each given a small deterministic extractor. 2,160 more facets
at W=13736; the record now runs from August 2013 and holds six pathology draws.

Three things learned on the scans. Lab ids must name the lab that did the work, because ACL absorbed
Healthscope and quotes its old lab numbers as history. A row that only a later report prints is
emitted from that report under the original draw's number, with a note saying so. And a text
extraction can be wrong where the scan is right: the 2024 ECG date was 10/03 in the text and 10/09
on paper, so the record was corrected and names the page that settled it.
