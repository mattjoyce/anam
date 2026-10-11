---
type: gap
title: Whether writing an id field counts as composing identity is not settled
description: "The contract says an importer composes no identity, and the gate does compose source_ref; but importers write the payload fields the identity pointers name: bp_csv's ordinal reading, garmin's id, the extractors' record ids."
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - ../anam-importers/bp_csv/run.py
  - ../anam-importers/records/run.py
  - docs/IMPORTERS.md
  - internal/def/check.go
---

The mechanism is as documented: identity is composed at the gate from `identity_key`. The reading the doc should settle is whether a payload field that exists only to make rows distinct (`reading`) is shaping or identity. `records/run.py` composes identity locally only to refuse duplicates within a run. See [the gate composes identity](/decisions/the-gate-composes-identity.md).
