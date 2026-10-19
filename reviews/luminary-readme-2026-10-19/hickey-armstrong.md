# README through Hickey × Armstrong

Reviewed `README.md` on branch `card-73-readme` at commit `b253db7`, against the code in the same
tree (`internal/daemon`, `gate`, `store/ledger`, `store/ops`, `fold`, `docs/ARCHITECTURE.md`).

## Findings

1. **Fix.** Line 29 and Status: "Anam can be rebuilt from them [the importers]". Not the whole
   truth. ARCHITECTURE §1 says the continuity (order, watermarks, anam-time) exists nowhere
   upstream, and the notes and facts saved over MCP come from source `conversation`, which has no
   importer to resend them. Say facts rebuild from importers, and what Anam authors and what the
   person says into it come back only from the backup.

2. **Fix.** Status: "any kind of delete" is not built. Verified: `anam retract --yes` and
   retirement tombstone a source's live facets through the gate (`internal/app/retract.go`,
   `retire.go`). A tombstone is an appended anam-event, so not a delete, but a stranger with a
   wrong fact is told nothing can be done. One sentence: a fact can be retracted, which drops it
   from the current value and never from the ledger.

3. **Fix.** The README names no failure. Verified in code: `commit` is idempotent (`gate.go:61`),
   the triggers refuse every update and delete (`ledger.go:15`), every client call carries a
   deadline and an `Anam-Timeout` header (`client.go:75,97`), and the operational store is replayed
   with no transaction across the two files. Give "How it fits together" a paragraph on the error
   kernel: ledger plus gate checks must not corrupt; a crashed importer re-runs; the second file
   can be behind, never wrong; with the daemon down, facts wait at the source for the next run.

4. **Consider.** Line 202: the second file "can be rebuilt by replaying the ledger". Only its
   derived tier is. ARCHITECTURE §8 lists an authored tier (registered templates, bookmarks, the
   operational log) that comes back from nowhere Anam can reach. Say "derived tables by replay;
   what the operator registered, from backup".

5. **Consider.** Nothing on a leaked token. Verified: the daemon holds only token hashes
   (`auth.go:17-19`), loads the importers file once at start (`daemon.go:138`), and an importer's
   token writes only its own source and types. So a leaked importer token means bad facts under
   one source, retractable; revocation is removing the hash and restarting. Two sentences under
   "Reads are narrowed by role".

6. **Consider.** Line 7: current state is "never stored". The `checkpoint` table (`ops.go:102`)
   and the facet index store it, as derived values. Write "derived from the facts; every cache of
   it is rebuilt by replay".

7. **Consider.** Lines 196-199: the diagram puts checkpoints as a stage between ledger and fold. A
   checkpoint is the fold's value at a watermark, off to the side. Draw it as a branch, so the
   record and its derivations are not one pipeline.

8. **Note.** Lines 40-84: the Definition tables hand-copy a place that moves
   (`internal/def/manifests/`); `check-links.sh` checks links, not this table. Generate it in the
   gate, or cut it to type names and point at the daemon.

9. **Note.** Status: "Nightly backups run" names no actor. The daemon only answers
   `POST /v1/backups`; something outside schedules it, and the config comment warns a backup on
   the record's disk shares its failure. Name who runs it and where.

10. **Note.** Claims that hold: loopback needs no token (`auth.go:57-66`); `init` and `daemon`
    refuse when a server is set (`app.go:149`); Definition versions are immutable
    (`register.go:18`); 27 bundled manifests.

## Where the two voices meet and part

They meet on the ledger. Hickey's value-not-place and Armstrong's error kernel are one object here:
an append-only file of immutable facts whose triggers refuse overwrite, everything else derived
and disposable. The README states the value side well (two clocks, versions beside the old, one
way in). Both lenses want the same addition: everything outside that file is a derivation you may
lose, and here is what each comes back from.

They part on what the README owes the reader. Hickey is content that it says what Anam is before
what it does; his complaints are braided words, "stored" for "derived", a cache drawn as a stage, a
table copying a moving place. Armstrong finds no failure named at all: not a down daemon, a crashed
importer, a leaked token, or the tier whose only other home is a backup. Hickey would cut lines;
Armstrong would add a paragraph. Add the paragraph first; it is what the stranger needs on the day
something breaks.
