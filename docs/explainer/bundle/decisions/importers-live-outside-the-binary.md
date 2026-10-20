---
type: decision
title: Importers live outside the binary, as ductile plugins
description: A source is a Python plugin in anam-importers that knows one vendor and shapes rows to a Definition; one generic anam_submit posts under a per-source token; Anam keeps the Definitions, the gate and the record and knows no vendor.
timestamp: 2026-10-11T00:00:00Z
resource: ../anam-importers/anam_importer/ductile.py#run
anchor: "request = json.load(sys.stdin)"
basis: docs
tier: core
alternatives:
  - Capture workers compiled into the binary that owns the ledger (how health-db, withings, bp-csv, records and documents began; withings and bp-csv moved out in cards 54 and 80, records and documents in card 119, and sharing a binary let their identity composition drift)
  - Ductile holding schemas or derived data, or Anam scheduling and polling (both refused, since each puts back the coupling the split removed)
affects:
  - /components/importer-base.md
  - /components/anam-submit.md
  - /components/daemon.md
  - /components/gate.md
  - /terms/importer.md
---

**Context.** The gateway (ductile) decides when to do things; Anam is the record of what was learned and when. Vendor table layouts inside Anam's binary braided what a source says with how Anam stores it.

**Choice.** An importer reads the native format, shapes payloads only, validates locally against the Definition it carries, cuts batches of 49, and emits an event; the gateway routes it to that importer's `anam_submit` instance, which holds the token. The contract is docs/IMPORTERS.md §8: the wire format, tokens, all-or-nothing batches, and what each status means. What is borrowed: Confluent's immutable numbered schema versions, CloudEvents' envelope field names, Splunk HEC's tokens pinned to sourcetypes, Elastic's integrations shipping their own schemas.

**Why.** The gate already owned the admission contract; moving the vendor out makes the code agree with ARCHITECTURE §2's "a pluggable client, not part of the kernel", and a crashed importer can only fail to send, never corrupt the record.

**What it costs.** Each source needs a resend-everything mode and a cut-over (the records and documents importers kept their sources' names, so they resend what Anam's own workers admitted as no-op; the Garmin importer replaced health-db under a new source name, so the old source was retired by tombstone). The two repos version separately.
