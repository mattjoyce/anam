---
type: data-structure
title: Config limits
description: The 21 bounds and waits the daemon, the gate, the inspector and the CLI client take from the loaded config, each a key with a shipped default and a range checked at start.
timestamp: 2026-10-17T00:00:00Z
resource: internal/config/limits.go#Limits
anchor: "type Limits struct {"
basis: code
tier: detail
group: contract
references: []
---

Card 151. `Limits` is squashed into `Config`, so its keys sit at the top level beside the [config keys](/data-structures/config-keys.md), each with an `ANAM_` variable (`ANAM_ACCEPT_WAIT`) and a `Default…` constant in `internal/config`, with the reason for the number beside it. `Limits.Validate` range-checks them at load and again at `daemon.Open`, so a zero wait never reaches a request.

| Key | Default | Bounds |
|---|---|---|
| `http_read_header_timeout` | 5 s | reading a request's headers |
| `http_read_timeout` | 30 s | reading a whole request, body included |
| `http_write_timeout` | 2 m | writing one response; the inspector's whole-record page is the largest |
| `http_idle_timeout` | 2 m | a kept-alive connection waiting for its next request |
| `shutdown_grace` | 10 s | in-flight requests at shutdown |
| `accept_wait` | 250 ms | an operator's write waiting for the [write queue](/decisions/one-write-queue-in-the-daemon.md) |
| `importer_wait` | 5 s | an importer's write waiting for the queue |
| `retry_after` | 15 s | what a 503 asks its caller to wait, at least one second |
| `mint_quiet` | 2 s | quiet before the [mint worker](/decisions/mint-cadence-follows-the-ledger.md) mints |
| `max_payload_bytes` | 300 KiB | one envelope's payload; a batch body is 49 of them plus 1 MiB |
| `commit_retries` | 5 | the gate's retries on a lost watermark race |
| `max_manifest_bytes` | 1 MiB | a registered Definition |
| `max_document_bytes` | 64 MiB | one posted document |
| `snapshot_ttl` | 24 h | an incomplete snapshot before it is dropped |
| `snapshot_keep` | 2 | complete snapshots kept per source |
| `max_snapshot_parts` | 100000 | parts one snapshot may declare |
| `max_absent_ids` | 1000 | absent facet ids a snapshot report lists (it still counts all) |
| `max_absent_judged` | 5000 | uncarried live facets a report judges, past which it is 413 |
| `max_response_bytes` | 64 MiB | what the [client](/components/client.md) reads of one answer |
| `network_margin` | 10 s | how much longer the client waits than it asks the daemon to take |
| `inspect_cached_reads` | 4 | as-of folds the [inspector](/components/inspect.md) keeps; 0 keeps none |

Every byte limit is capped at 1 GiB. `timeout`, `ingest_timeout` and `backup_keep` became `Default…` constants in the same change, and `library_folder_limit` and `checkpoint_budget` already were. `DefaultLimits()` is every limit at its default, for a test or a tool that loads no config. Values that are not here are invariants (see [a hard-coded value is a default or an invariant](/decisions/a-hard-coded-value-is-a-default-or-an-invariant.md)).
