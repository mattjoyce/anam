---
type: gap
title: A mint that fails after an importer batch is reported only in the daemon log
description: mintOnRequest discards the result of mintAtHead, so a failed background mint reaches neither the batch's response nor GET /v1/status; only an operator mint over POST /v1/checkpoints returns its error.
timestamp: 2026-10-14T00:00:00Z
looked_at:
  - internal/daemon/minting.go
  - internal/daemon/envelopes.go
  - internal/daemon/api.go
---

`mintAtHead` returns an `api.MintRun` with `Error` and logs the failure; `mintOnRequest` calls it as a statement. Until card 119 a declared ingest kept that result on its operation as `mint.error`; with `POST /v1/ingest` gone nothing keeps it. The next changing batch or a restart (`mintIfBehind`) retries, and reads fold in full meanwhile, so nothing is wrong, only unseen. See [mint cadence follows the ledger](/decisions/mint-cadence-follows-the-ledger.md).
