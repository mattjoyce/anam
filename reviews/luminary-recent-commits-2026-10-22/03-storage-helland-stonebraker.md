# Storage changes, cards 215 and 216: Helland x Stonebraker (specialization x semantics)

After pass, code audit of `git diff f3f0ce5 6489310` (store, mint, serve, receipt, app, ARCHITECTURE.md). Sized to one person's record: 57 MB ledger, 13,377 live facets, 47 checkpoint values, full fold 1.3 s, month read 0.16 s with checkpoints and 1.47 s without.

## Verdict

Sound. Files to rows keeps the two-file rule: the mint and the serve path open the ledger read-only and write only the operational store, `move-artifacts` reads a folder it never changes and writes rows one idempotent `Put` at a time, and a read that meets a missing checkpoint declines and folds. A crash leaves the store behind, never wrong. Identity is the content hash, not the row id. The authored tier is listed explicitly in §8. One small defect, two design calls for Matt, two points fine for one person.

## Findings, ranked

### 1. Defect. The schema promises a delete guard on `checkpoint_value` that does not exist

`internal/store/ops/ops.go:146-150` says "a delete is refused while any checkpoint row still names the value". The only trigger is `checkpoint_value_no_update` at line 156. What actually keeps a named value from going is the order and predicate of three statements in `internal/store/ops/collect.go:31-37`. Stonebraker: a constraint the engine could hold is held in application code, and the schema comment, where AGENTS.md says tier claims belong, is false. Effect today is nil: one writer, three statements, one transaction. Fix: a `BEFORE DELETE ON checkpoint_value` trigger that raises when `EXISTS (SELECT 1 FROM checkpoint WHERE value_hash = OLD.hash)`, and the comment becomes true.

### 2. Design call for Matt. The next schema bump strands the authored content rows

`ops.go:455` tells the operator to set a refused file aside, re-run init, and carry rows with `move-artifacts --registered-from`. `internal/store/ops/registered.go:41` and `:114` carry only `registered_definition` and `registered_projection`. Receipts, served values, saved selections and their codes now live in that same file, and nothing in the binary copies them from a version-9 file into a version-10 one. Before card 215 they were files and survived every bump untouched. Helland: this is exactly the data a rebuild cannot recover, and the written repair forgets it. The 8 bump was carried by hand with ATTACH; the 10 bump is the first where forgetting costs receipts. Fix, small: extend `ReadRegistered`/`CarryRegistered` to the four authored content tables (one loop over `Kinds()` minus checkpoint, same two-column shape), so the documented repair carries the whole authored tier.

### 3. Design call. Listing receipts is a full scan, re-hash and JSON parse per row to filter on two fields

`internal/serve/serve.go:467-489` lists every receipt digest, `Get`s each (which re-hashes, `content.go:144`), parses it, then applies `admits`. A timeline receipt carries every served facet id (`serve.go:212`): about 0.5 MB for the whole record, 15 KB for a month. A recording token (card 216) leaves one per read, so the table grows with reads, not with the record. Cost is linear in total receipt bytes. Fine today. At ten recorded month reads a day, a year's list is about 55 MB read, hashed and parsed per `GET /v1/receipts`, under a second on the NAS; a few whole-record reads a day makes it gigabytes and seconds. Stonebraker: the store should answer `principal` and `served_at` with an index. Smallest fix: a derived `receipt_index (hash, served_at, principal)` written in the same transaction as the receipt row (same file, so one transaction is honest), rebuilt from `receipt` when empty; `List` filters in SQL and hashes only what it returns.

### 4. Fine for one person. Re-hashing on read is a second pass, not a design error

`ReadWindow` already streams every bucket's bytes through SHA-256 for the whole value's hash (`internal/mint/window.go:239-262`), so `Get`'s check is a second hash of bytes already in memory: 47 values, at most 1 MiB each, well under 100 ms worst case, inside the measured 0.16 s. The check is what turns a damaged row into an error instead of a wrong answer. Taste: `Get` could return the digest it computed so `readBucket` hashes once. Not worth a card.

### 5. Design call. The checkpoint machinery is sized for a record forty times this one

`checkpoint_budget` 1 MiB was priced at 46 ms/MB re-derive on the NAS; the year-to-month-to-day layout at 600k facets (19.8 s full fold against 132 ms). Both are measurements. Today's numbers say what it buys here: 1.3 s per read, 1.47 s to 0.16 s on the inspector's month view. The digest, decline-and-fold, write-back, collection and move command are correct and oracle-tested, and exist to save 1.3 s. Stonebraker's reading: for this record the cache could be dropped and only the month view would notice. That is not an argument to remove it, but to stop adding to it and watch one number, the full fold, which needs nothing until it passes a few seconds.

## Where the two lenses pull apart

Helland is satisfied: immutable tables carry triggers, the cache is marked a cache, every cross-file step is idempotent and re-runnable, the authored list is explicit. His one worry is finding 2. Stonebraker is less satisfied: findings 1 and 3 are a constraint and a query the engine should own and the application holds, and finding 5 is a cache built for a workload the record does not yet have. Helland's discipline is why the cache can be dropped safely; Stonebraker's measurement is why it should not be extended.
