# Collation: recent commits, 2026-10-22

Three After passes on anam master 441e4a0 to 6489310 (cards 215, 216, the query fixes and the gosec
hardening), each run alone on Fable: Hickey x Armstrong on card 215 (01), Hickey x Armstrong then
Nissenbaum x Solove on card 216 and the small fixes (02), Helland x Stonebraker on the storage move
(03). Convergence across lenses is the strongest signal; tensions are kept, not averaged.

## Verdict

Sound and live. The error kernel (ledger, gate) has no lines in the range. Every loss path on the
new checkpoint cache ends in "slow, never wrong". Two reviews independently rank the same latent
defect first. Two real code defects sit in card 216 and the number parser. The rest is design calls
for Matt and taste.

## Converged findings, ranked

1. **Defect, latent. The next schema bump loses receipts, served values and saved selections.**
   Found by 01 and 03. The carry (`internal/store/ops/registered.go` ReadRegistered, CarryRegistered)
   copies only registered Definitions and projections. The four authored content tables now live in
   the same file and nothing carries them; this bump had the artifacts folder, the next has none.
   Fix: extend the carry to the four authored tables, in the same transactions, and name them in
   the refusal's remedy text.

2. **Defect. The exact-decimal parser can panic.** Found by 02, reproduced. `internal/query/number.go`
   around line 97: `len(digits)+exp` overflows on an exponent like `1e9223372036854775807`, the guard
   reads false and `strings.Repeat` panics. Any reader can send it. Fix: refuse an exponent beyond a
   stated bound before any arithmetic.

3. **Defect. A recording token's read fails as 500 when the store is busy.** Found by 02. Filing
   goes through the write handle outside the write queue (`internal/daemon/api.go` ~272,
   `query.go` ~76); a mint or backup holding its transaction leaves the Put to the 5 s busy timeout
   and the read dies as 500. Fix: map SQLite busy to 503 in `writeReadError`, as checkpoints do.

4. **Mismatch. `checkpoint_value`'s comment promises a delete guard that is not there.** Found by 01
   and 03. The reviews split on the fix: 03 says add the trigger, 01 says delete the clause because
   the tests model a lost checkpoint with a plain DELETE and Matt decided checkpoints may go
   missing. Lead's call: delete the clause; a guard would fight the cache design.

5. **Design call. dry_run is the unrecorded payload read.** 02. A recording token gets a query's
   full answer unrecorded with one flag, the flag anam-mcp's query tool sends. Refuse dry_run for
   recording tokens (breaks the mcp try-a-program path for such tokens) or accept the hole.

6. **Design call. Any role lists every reader's receipts.** 02, where Nissenbaum and Solove agree:
   who may list is a role rule, never a content judgement. Clamp a recording token's list to its
   own; the operator lists all.

7. **Design call. Listing receipts scans, re-hashes and parses every row.** 03. Fine today; grows
   with reads once a token records. When it matters: a derived `receipt_index (hash, served_at,
   principal)` written in the same transaction, rebuilt when empty.

8. **Gap. Authored tables refuse UPDATE and DELETE but not INSERT OR REPLACE.** 01. The ledger has
   that trigger; these tables rely on the hash check at read, which detects tampering, never serves
   it. Add the trigger if wanted; `opstest.Corrupt` loses its route.

9. **Design call. Rollback past 6489310 is closed once a recording token has read.** 02. An older
   binary refuses a named receipt and the list fails whole. Record it; leave the code.

10. **Taste.** `picks` is a place in an append-only log, and LogSealed answers two questions with
    one bit (01, leave it). A local `anam checkpoint` may write beside the daemon unannounced (01).
    GET /v1/timeline answers two shapes by identity (02). `safepath.ResolveFile`'s comment overclaims
    confinement (02). `inspect/url.go` rewrites `url.Values.Encode` (02). Re-hashing on read is a
    second pass over bytes already in memory (03, fine). The checkpoint machinery is sized for a
    record forty times this one (03): stop adding to it and watch the full-fold time.

## Where the lenses pulled apart

Hickey wants the authored rows and the cache in separate files so "delete and replay" reaches no
authored fact; Armstrong wants one backup unit and one writer. Card 215 chose Armstrong, and
finding 1 is the bill. Helland is satisfied by the immutability discipline; Stonebraker holds that
findings 4 and 7 are a constraint and a query the engine should own. On card 216, Hickey wants the
read path untouched by identity and Armstrong wants the record of a read as reliable as the read;
the code chose Armstrong and paid in finding 3.
