# Card 215 through Hickey x Armstrong (After pass)

Artifact: `git diff f3f0ce5 e02da49 -- internal cmd docs/ARCHITECTURE.md`, merged to master and live 2026-10-22.

## Verdict

Sound. The error kernel is untouched: `internal/store/ledger` and `internal/gate` have no lines in this range. Every loss path I could construct ends in "slow, never wrong": a read declines and folds the ledger, a mint writes the value back, a backup is one `VACUUM INTO` per file. The one real defect is latent, in the path that carries the store across the next schema bump. The rest is a false comment, a guard the ledger has and these tables lack, and two matters of taste.

## Findings, ranked

**1. The schema-bump carry drops the new authored rows. Real defect, latent.**
`internal/store/ops/registered.go:22-37` defines `Registered` as "what one operational store holds that its ledger cannot give back", and `ReadRegistered` (line 41) and `CarryRegistered` (line 114) copy registered Definitions and projections only. The store now also holds receipts, served values, saved selections and selection codes, which come back from nowhere (§8). `anam move-artifacts --registered-from` (`internal/app/movefiles.go:118-140`) and the remedy text in `pinSchemaVersion` are the documented route across a bump; this time the `--from <folder>` leg carried the content. At schema 10 there is no folder: the carry would silently leave every receipt behind and the daemon would answer "no receipts" from a store that looks whole. Smallest fix: extend `Registered` with the four authored content kinds, in the same transactions, and name them in the remedy text. (ARCHITECTURE line 36 still says checkpoints are "backed up rather than assumed rebuildable"; the card already flags that for Matt.)

**2. A guard the comment promises does not exist. Real mismatch, fix the comment.**
`ops.go:147-148` says a delete of `checkpoint_value` "is refused while any checkpoint row still names the value". The only trigger on the table is `checkpoint_value_no_update` (line 156). `opstest.LoseCheckpointValue` deletes a named value with a plain `DELETE`, and `internal/mint/collect_test.go` depends on that to model the cache miss the card was built for. The code and §8 ("lets a mint delete a value it no longer reads") are right; the comment is wrong. Delete the clause.

**3. "Immutable for every writer" is weaker than the ledger's. A gap, not a defect.**
The four authored tables refuse `UPDATE` and `DELETE` (`ops.go:259-307`) but not an insert over an existing key. The ledger adds `facet_no_replace BEFORE INSERT ... WHEN EXISTS` (`internal/store/ledger/schema.go:101-104`) because `INSERT OR REPLACE` deletes without firing the delete trigger, and §8 says so. `opstest.Corrupt` uses exactly that route to plant a tampered receipt. The hash catches it on read (`Get` returns `ErrCorrupt`), so the failure is detected, never served. Fix if wanted: one `BEFORE INSERT ... WHEN EXISTS (SELECT 1 FROM receipt WHERE hash = NEW.hash AND body IS NOT NEW.body)` trigger per authored table; `Put`'s idempotent re-put still passes. The cost is that `Corrupt` loses its route.

**4. Two writers on one store are allowed and unannounced. Taste.**
A local `anam checkpoint` with no server (`internal/app/checkpoint.go:84-100`) and `anam move-artifacts` open the live store for writing beside the daemon. A mint writes a value then its row as two statements (`internal/mint/mint.go:262-270`); the daemon's collect (`collect.go:37`) can delete that value in between, leaving a row that names nothing. Recovery is the designed one: the read declines (`read.go:110`), the next mint's `Size` check (`incremental.go:128`, `mint.go:248`) writes it back, and two mints at one `(span, v)` hit the primary key. Nothing corrupts. `move-artifacts` says "stop the daemon first"; `checkpoint` could say the same, or refuse.

**5. `picks` is a place, and `LogSealed` answers two questions with one bit. Taste, acceptable.**
`AppendMintRun` (`checkpoint.go:113`) overwrites `mint_run.picks` at the same `w`. Hickey calls that a place in a log that was append-only; Armstrong notes the only writer is a mint at that same `w` on the write queue, and `RunCheckpoints` (line 139) reads digest and rows in one read transaction, so no reader sees a torn pair. `LogSealed` (lines 170-190) braids "did the last mint finish" with "is the cache intact"; both answers are "fold in full", so un-braiding adds a path with nothing to do. Leave it.

## The two voices

**Hickey.** The move un-braids where the bytes live from what they mean: five tables of one shape, the mapping quarantined in `content.go`. The seal is now derived (a digest of the log, compared on read) instead of stored. The derived tier stays provably derived: `checkpoint_value` is insert-only, deletable, rebuilt by minting, and the rebuild test still drops the tier. What is left complected is the `kind` string that selects a table at run time, and `picks` as a place. Neither costs correctness.

**Armstrong.** Each new path names its failure and recovery, and the tests exercise the failures: `collect_test.go` (store) covers keep, decline, orphans, a crashed mint's rows and no run; `mint/collect_test.go` holds every watermark to the fold, loses a value, a row and a head with an older version left, loses them mid-read for `ReadAt` and `ReadWindow`, and proves the write-back for full, incremental and with a tail. Collection runs after the run is recorded, so a crash leaves the log longer, never shorter than the run. Not tested: the carry in finding 1, and `INSERT OR REPLACE` in finding 3.

**Where they converge.** Nothing in this diff can corrupt; everything that goes missing makes a read slower and a mint write more. The kernel is untouched.

**Where they pull apart.** Hickey wants the authored rows and the cache in separate files, so "delete the file and replay" (§8) stays a repair that reaches no authored fact. Armstrong wants one backup unit and one writer, which is what the card bought. The card chose Armstrong, and finding 1 is the bill: once authored rows share a file with the derived tier, every rebuild path has to carry them, and one does not yet.
