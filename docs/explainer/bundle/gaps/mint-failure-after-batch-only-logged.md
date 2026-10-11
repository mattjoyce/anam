---
type: gap
title: A mint that fails after an importer batch is reported only in the daemon log
description: After a declared ingest the mint's error lands on the operation as mint.error; after an importer batch mintOnRequest discards the result, and neither the response nor GET /v1/status carries it.
timestamp: 2026-10-11T00:00:00Z
looked_at:
  - internal/daemon/ingest.go
  - internal/daemon/envelopes.go
  - internal/daemon/api.go
---

`mintAfterIngest` returns an `api.MintRun` with `Error`; `runIngest` stores it on the operation, `mintOnRequest` calls it as a statement. The next changing batch or a restart (`mintIfBehind`) retries, and reads fold in full meanwhile, so nothing is wrong, only unseen. See [mint cadence follows the ledger](/decisions/mint-cadence-follows-the-ledger.md).
