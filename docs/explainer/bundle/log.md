# Log

## 2026-10-11

First build of the bundle, at anam commit 38aa3dad70a7d1eda90193675073ffe75b602073 (master) and anam-importers commit 02a4907a980ff186bc21c85bdea966bd714c3938 (branch card-58-one-off).

- Code: nothing changed; this is the baseline. The repo at this commit has the daemon owning the ledger on unraid, importers for Withings, blood pressure, DreamStation and records in the sibling repo, retraction by complete resend and by retirement, composed reads from the last mint run, and bounded reads over the daemon.
- Bundle: one purpose, 29 terms, 29 components, 6 workflows, 17 data structures, 21 decisions, 27 invariants, 5 change paths, 16 gaps.

## 2026-10-13

Card 50 closes the importers move, at anam master after 3b97178.

- Code: `internal/capture/healthdb` deleted, with its `scanImport` case, scope entry and example import; health-db itself had left the live imports and its facets were retired on 2026-10-11 (card 55). `records` and `documents` remain in process until card 119.
- Bundle: the component `capture-healthdb` and the gap `healthdb-worker-still-built-in` removed, and the four pages that depended on the component no longer list it. `docs/ARCHITECTURE.md` §2 now draws importers, not capture workers.

## 2026-10-13, card 21

- Code: the inspector's timeline has a checkpoint strip and a header line counting sealed, behind and never-minted buckets; `inspect.Handler` takes a `Checkpoints` log.
- Bundle: the inspector component names the strip and depends on mint and the ops store.
